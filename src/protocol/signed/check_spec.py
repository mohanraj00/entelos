#!/usr/bin/env python3
"""Check the signed record vectors without a Host.

The checker applies the rules of SIGNED.md to each case, with its own
Ed25519 code. A pass shows that the vectors, the schemas and the rules agree.
The Go verifier runs the same vectors in src/lifecycle-core/sign_test.go.
"""
import argparse
import base64
import copy
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import tempfile

HERE=Path(__file__).resolve().parent
# test_protocol makes a work directory at import when this is not set. The checker writes nothing there.
os.environ.setdefault('PROTOCOL_TEST_WORK',tempfile.gettempdir())
sys.path.insert(0,str(HERE.parent));sys.path.insert(0,str(HERE))
from test_protocol import SchemaError, validate  # noqa: E402
import make_vectors  # noqa: E402
import ref_ed25519 as ed  # noqa: E402

SIGNED=json.loads((HERE/'schema'/'signed.schema.json').read_text())
KEYSET=json.loads((HERE/'schema'/'keyset.schema.json').read_text())
SPEC=(HERE/'SIGNED.md').read_text()
RECORD_FORMAT=re.compile(r'entelos\.record/([1-9][0-9]{0,8})')
KEYSET_FORMAT=re.compile(r'entelos\.keyset/([1-9][0-9]{0,8})')
DOMAIN=b'entelos.signature/1'

class Refusal(Exception):
    def __init__(self,code,rule=None):
        self.result={'valid':False,'code':code}
        if rule:self.result['rule']=rule

def table_terms(text):
    """The codes and the (code, rule) pairs in the Verification section of SIGNED.md."""
    section=text.split('\n## Verification\n',1)[1].split('\n## ',1)[0]
    pairs=set(re.findall(r'`([a-z_]+)`, rule `([a-z_]+)`',section))
    codes=set(re.findall(r'\| `([a-z_]+)`',section))
    return codes,pairs

def decode64(value):
    """Canonical base64 with padding. None for each other text."""
    try:raw=base64.b64decode(value,validate=True)
    except ValueError:return None
    return raw if base64.b64encode(raw).decode()==value else None

def strict_object(raw):
    """A strict JSON object: UTF-8, no duplicate name, no NaN or Infinity. None for each other text."""
    def pairs(items):
        if len({k for k,_ in items})!=len(items):raise ValueError('duplicate name')
        return dict(items)
    def constant(name):raise ValueError(name)
    try:value=json.loads(raw.decode('utf-8'),object_pairs_hook=pairs,parse_constant=constant)
    except (ValueError,RecursionError):return None
    return value if isinstance(value,dict) else None

def record_format(doc,rule):
    if 'format' not in doc:raise Refusal('unsupported_record_format')
    m=RECORD_FORMAT.fullmatch(doc['format']) if isinstance(doc['format'],str) else None
    if not m:raise Refusal('invalid_signed_record',rule)
    if m.group(1)!='1':raise Refusal('unsupported_record_format')

def read_key_set(doc):
    """Rules K1 to K7 of SIGNED.md. Return the keys by key_id."""
    if not isinstance(doc,dict):raise Refusal('invalid_keyset','document')
    if 'format' not in doc:raise Refusal('unsupported_keyset_format')
    m=KEYSET_FORMAT.fullmatch(doc['format']) if isinstance(doc['format'],str) else None
    if not m:raise Refusal('invalid_keyset','format')
    if m.group(1)!='1':raise Refusal('unsupported_keyset_format')
    try:validate(KEYSET,doc,'keyset',KEYSET['$defs'])
    except SchemaError:raise Refusal('invalid_keyset','schema') from None
    keys={}
    for entry in doc['keys']:
        if entry['algorithm']!='ed25519':raise Refusal('unsupported_algorithm')
        public=decode64(entry['public_key'])
        if public is None or len(public)!=32:raise Refusal('invalid_keyset','key_encoding')
        if 'ed25519:'+hashlib.sha256(public).hexdigest()!=entry['key_id']:raise Refusal('invalid_keyset','key_id')
        if entry['key_id'] in keys:raise Refusal('invalid_keyset','duplicate_key')
        keys[entry['key_id']]=(public,entry['status'])
    return keys

def verify(doc,key_set):
    """Rules K1 to K7, then S1 to S12 of SIGNED.md. Return the result in the vector shape."""
    keys=read_key_set(key_set)
    if not isinstance(doc,dict):raise Refusal('invalid_signed_record','document')
    record_format(doc,'format')
    try:validate(SIGNED,doc,'signed',SIGNED['$defs'])
    except SchemaError:raise Refusal('invalid_signed_record','schema') from None
    if doc['algorithm']!='ed25519':raise Refusal('unsupported_algorithm')
    raw=decode64(doc['record'])
    if raw is None:raise Refusal('invalid_signed_record','record_encoding')
    signature=decode64(doc['signature'])
    if signature is None or len(signature)!=64:raise Refusal('invalid_signed_record','signature_encoding')
    envelope=('application/vnd.cel-lifecycle.'+doc['record_kind']+'+json').encode()+b'\0'+raw
    if 'sha256:'+hashlib.sha256(envelope).hexdigest()!=doc['record_ref']:raise Refusal('record_mismatch','ref')
    if doc['key_id'] not in keys:raise Refusal('unknown_key')
    public,status=keys[doc['key_id']]
    if status=='revoked':raise Refusal('revoked_key')
    if not ed.verify(public,DOMAIN+b'\0'+envelope,signature):raise Refusal('signature_invalid')
    record=strict_object(raw)
    if record is None:raise Refusal('invalid_signed_record','record_json')
    record_format(record,'record_format')
    if record.get('scope')!=doc['scope'] or record.get('function')!=doc['function']:raise Refusal('record_mismatch','binding')
    return {'valid':True,'record_kind':doc['record_kind'],'record_ref':doc['record_ref'],'signer':key_set['signer'],
            'key_id':doc['key_id'],'key_status':status}

def judge(doc,key_set):
    try:return verify(doc,key_set)
    except Refusal as r:return r.result

REQUIRED=('valid-approval','valid-activation','changed-record','unknown-key','revoked-key','format-major-2','record-format-major-2')

def run(doc):
    errors=[];names=set();codes_seen=set();pairs_seen=set()
    codes,pairs=table_terms(SPEC)
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']:
        name=case['name']
        if name in names:errors.append(name+': duplicate case name')
        names.add(name)
        expect=case['expect']
        if case['key_set'] not in doc['key_sets']:
            errors.append(name+': unknown key set');continue
        got=judge(case['signed'],doc['key_sets'][case['key_set']])
        if got!=expect:errors.append(name+': expected '+json.dumps(expect,sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        if expect.get('code'):codes_seen.add(expect['code'])
        if expect.get('rule'):pairs_seen.add((expect['code'],expect['rule']))
    errors+=['required case is absent: '+n for n in REQUIRED if n not in names]
    errors+=['code without a case: '+c for c in sorted(codes-codes_seen)]
    errors+=['rule without a case: %s, rule %s'%p for p in sorted(pairs-pairs_seen)]
    errors+=['code not in SIGNED.md: '+c for c in sorted(codes_seen-codes)]
    errors+=['rule not in SIGNED.md: %s, rule %s'%p for p in sorted(pairs_seen-pairs)]
    # A valid record of the vectors has the canonical bytes of its record object.
    for kind,record in doc['records'].items():
        if strict_object(make_vectors.canonical(record))!=record:errors.append(kind+': the record is not canonical')
    return errors

def self_test(doc):
    """Change a copy of the cases. The checker must report each change."""
    def case(d,n):return next(c for c in d['cases'] if c['name']==n)
    def accept_changed(d):case(d,'changed-record')['expect']=case(d,'valid-activation')['expect']
    def accept_unknown_key(d):case(d,'unknown-key')['expect']['code']='signature_invalid'
    def accept_revoked(d):case(d,'revoked-key')['expect']={'valid':True}
    def unrevoke(d):d['key_sets']['current']['keys'][2]['status']='retired'
    def retire_as_active(d):case(d,'valid-retired-key')['expect']['key_status']='active'
    def change_valid_record(d):
        s=case(d,'valid-approval')['signed'];raw=bytearray(base64.b64decode(s['record']));raw[20]^=1
        s['record']=base64.b64encode(bytes(raw)).decode()
    def change_valid_signature(d):
        s=case(d,'valid-activation')['signed'];raw=bytearray(base64.b64decode(s['signature']));raw[63]^=128
        s['signature']=base64.b64encode(bytes(raw)).decode()
    def major_as_schema(d):case(d,'format-major-2')['expect']={'valid':False,'code':'invalid_signed_record','rule':'format'}
    def inner_major_accepted(d):case(d,'record-format-major-2')['expect']=case(d,'valid-approval')['expect']
    def drop_code_case(d):d['cases']=[c for c in d['cases'] if c['expect'].get('code')!='revoked_key']
    def drop_rule_case(d):d['cases']=[c for c in d['cases'] if c['expect'].get('rule')!='duplicate_key']
    def swap_order(d):case(d,'ref-before-key')['expect']={'valid':False,'code':'unknown_key'}
    def new_rule(d):case(d,'extra-field')['expect']['rule']='fields'
    missed=[]
    for f in (accept_changed,accept_unknown_key,accept_revoked,unrevoke,retire_as_active,change_valid_record,change_valid_signature,
              major_as_schema,inner_major_accepted,drop_code_case,drop_rule_case,swap_order,new_rule):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    path=HERE/'vectors'/'signed-cases.json'
    doc=json.loads(path.read_text())
    errors=ed.self_test()
    if path.read_text()!=make_vectors.text():errors.append('vectors/signed-cases.json is not the output of make_vectors.py')
    errors+=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'cases':len(doc['cases']),'errors':errors,
                      'scope':'Signed record vectors: schemas, rule order, refusal codes and Ed25519 signatures. No Host ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
