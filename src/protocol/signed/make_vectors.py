#!/usr/bin/env python3
"""Build the signed record vectors from public test seeds.

A person wrote each expected result from SIGNED.md. No verifier output made
them. check_spec.py checks that vectors/signed-cases.json is the output of
this file. Use --write after an intended change.
"""
import argparse
import base64
import copy
import hashlib
import json
from pathlib import Path
import sys

HERE=Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import ref_ed25519 as ed  # noqa: E402

DOMAIN=b'entelos.signature/1'
OUT=HERE/'vectors'/'signed-cases.json'

def b64(data):return base64.b64encode(data).decode()
def ref(text):return 'sha256:'+hashlib.sha256(text.encode()).hexdigest()
def media(kind):return ('application/vnd.cel-lifecycle.'+kind+'+json').encode()
def canonical(record):
    """The record bytes of the Host for a record with ASCII strings only."""
    return (json.dumps(record,sort_keys=True,separators=(',',':'))+'\n').encode()
def record_ref(kind,raw):return 'sha256:'+hashlib.sha256(media(kind)+b'\0'+raw).hexdigest()
def message(kind,raw):return DOMAIN+b'\0'+media(kind)+b'\0'+raw

# Public test seeds. Never use these keys outside the vectors.
SEEDS={name:hashlib.sha256(('entelos signed record test key '+name).encode()).digest() for name in ('a','b','c','d')}
PUBLIC={name:ed.public_key(seed) for name,seed in SEEDS.items()}
KEY_ID={name:'ed25519:'+hashlib.sha256(public).hexdigest() for name,public in PUBLIC.items()}

def key(name,status,**extra):
    return dict({'key_id':KEY_ID[name],'algorithm':'ed25519','public_key':b64(PUBLIC[name]),'status':status},**extra)

def key_set(*keys):return {'format':'entelos.keyset/1','signer':'example-signer','keys':list(keys)}

WINDOW={'function':'record.visible','scope':'demo','from':'2026-11-01','until':'2027-01-01'}
DECISION=dict(WINDOW,format='entelos.record/1',snapshot_ref=ref('snapshot'),report_ref=ref('report'),actor='reviewer-a',
              reason='the report passed',disposition='accept',runner_policy='example-runner',decided_at='2026-11-15T10:00:00Z')
EVENT=dict(WINDOW,format='entelos.record/1',kind='enable',snapshot_ref=ref('snapshot'),report_ref=ref('report'),
           decision_ref=ref('decision'),targets=[],actor='reviewer-a',event_id='activate-1')

def signed(kind,raw,name,scope='demo',function='record.visible'):
    """The signed record of the record bytes, with the test key of the name."""
    return {'format':'entelos.record/1','scope':scope,'function':function,'record_kind':kind,'record_ref':record_ref(kind,raw),
            'record':b64(raw),'algorithm':'ed25519','key_id':KEY_ID[name],'signature':b64(ed.sign(SEEDS[name],message(kind,raw)))}

def ok(doc,status='active'):
    return {'valid':True,'record_kind':doc['record_kind'],'record_ref':doc['record_ref'],'signer':'example-signer',
            'key_id':doc['key_id'],'key_status':status}
def no(code,rule=None):
    out={'valid':False,'code':code}
    if rule:out['rule']=rule
    return out

def build():
    approval=signed('decision',canonical(DECISION),'a')
    activation=signed('event',canonical(EVENT),'a')
    def edit(base,**fields):
        doc=copy.deepcopy(base);doc.update(fields)
        return {k:v for k,v in doc.items() if v is not None}
    later=canonical(dict(EVENT,until='2028-01-01'))
    flipped=bytearray(base64.b64decode(activation['signature']));flipped[5]^=1
    as_event=canonical(DECISION)
    sets={
        'current':key_set(key('a','active'),key('b','retired',note='rotated out on 2026-11-01'),key('c','revoked',note='the private key leaked')),
        'format-major-2':dict(key_set(key('a','active')),format='entelos.keyset/2'),
        'format-missing':{k:v for k,v in key_set(key('a','active')).items() if k!='format'},
        'format-malformed':dict(key_set(key('a','active')),format='entelos.keyset/one'),
        'not-an-object':[key('a','active')],
        'no-keys':key_set(),
        'extra-field':dict(key_set(key('a','active')),owner='example'),
        'unknown-status':key_set(key('a','trusted')),
        'other-algorithm':key_set(dict(key('a','active'),algorithm='ed448')),
        'short-key':key_set(dict(key('a','active'),public_key=b64(PUBLIC['a'][:31]))),
        'key-with-white-space':key_set(dict(key('a','active'),public_key=b64(PUBLIC['a'])+'\n')),
        'key-id-of-another-key':key_set(dict(key('a','active'),public_key=b64(PUBLIC['d']))),
        'duplicate-key':key_set(key('a','active'),key('a','retired')),
    }
    cases=[
        ('valid-approval','current',approval,ok(approval)),
        ('valid-activation','current',activation,ok(activation)),
        ('valid-retired-key','current',signed('decision',canonical(DECISION),'b'),ok(signed('decision',canonical(DECISION),'b'),'retired')),
        # A changed record.
        ('changed-record','current',edit(activation,record=b64(later)),no('record_mismatch','ref')),
        ('changed-record-and-ref','current',edit(activation,record=b64(later),record_ref=record_ref('event',later)),no('signature_invalid')),
        ('changed-signature','current',edit(activation,signature=b64(bytes(flipped))),no('signature_invalid')),
        ('changed-kind','current',edit(approval,record_kind='event',record_ref=record_ref('event',as_event)),no('signature_invalid')),
        ('changed-scope','current',edit(approval,scope='other'),no('record_mismatch','binding')),
        ('changed-function','current',edit(approval,function='record.hidden'),no('record_mismatch','binding')),
        # Keys.
        ('unknown-key','current',signed('event',canonical(EVENT),'d'),no('unknown_key')),
        ('revoked-key','current',signed('event',canonical(EVENT),'c'),no('revoked_key')),
        ('revoked-key-before-signature','current',edit(signed('event',canonical(EVENT),'c'),signature=b64(bytes(flipped))),no('revoked_key')),
        ('key-id-of-another-key','current',edit(approval,key_id=KEY_ID['b']),no('signature_invalid')),
        ('ref-before-key','current',edit(signed('event',canonical(EVENT),'d'),record=b64(later)),no('record_mismatch','ref')),
        # Format versions. An unknown major version fails closed.
        ('format-major-2','current',edit(approval,format='entelos.record/2'),no('unsupported_record_format')),
        ('format-missing','current',edit(approval,format=None),no('unsupported_record_format')),
        ('format-malformed','current',edit(approval,format='entelos.record/one'),no('invalid_signed_record','format')),
        ('format-not-a-string','current',edit(approval,format=1),no('invalid_signed_record','format')),
        ('record-format-major-2','current',signed('decision',canonical(dict(DECISION,format='entelos.record/2')),'a'),no('unsupported_record_format')),
        ('record-format-missing','current',signed('decision',canonical({k:v for k,v in DECISION.items() if k!='format'}),'a'),
         no('unsupported_record_format')),
        ('record-format-malformed','current',signed('decision',canonical(dict(DECISION,format='record/1')),'a'),
         no('invalid_signed_record','record_format')),
        # The document.
        ('not-an-object','current',[approval],no('invalid_signed_record','document')),
        ('extra-field','current',edit(approval,signed_at='2026-11-15T10:00:00Z'),no('invalid_signed_record','schema')),
        ('missing-key-id','current',edit(approval,key_id=None),no('invalid_signed_record','schema')),
        ('unsigned-kind','current',signed('receipt',canonical(DECISION),'a'),no('invalid_signed_record','schema')),
        ('other-algorithm','current',edit(approval,algorithm='ed448'),no('unsupported_algorithm')),
        ('record-with-white-space','current',edit(approval,record=approval['record'][:40]+'\n'+approval['record'][40:]),
         no('invalid_signed_record','record_encoding')),
        ('record-not-canonical-base64','current',edit(approval,record='QR=='),no('invalid_signed_record','record_encoding')),
        ('short-signature','current',edit(approval,signature=b64(base64.b64decode(approval['signature'])[:32])),
         no('invalid_signed_record','signature_encoding')),
        ('record-not-an-object','current',signed('decision',b'[1]\n','a'),no('invalid_signed_record','record_json')),
        ('record-duplicate-name','current',signed('decision',canonical(DECISION)[:-2]+b',"scope":"demo"}\n','a'),
         no('invalid_signed_record','record_json')),
        ('record-not-utf8','current',signed('decision',canonical(DECISION).replace(b'reviewer-a',b'reviewer-\xff'),'a'),
         no('invalid_signed_record','record_json')),
        # The key set.
        ('keyset-format-major-2','format-major-2',approval,no('unsupported_keyset_format')),
        ('keyset-format-missing','format-missing',approval,no('unsupported_keyset_format')),
        ('keyset-format-malformed','format-malformed',approval,no('invalid_keyset','format')),
        ('keyset-not-an-object','not-an-object',approval,no('invalid_keyset','document')),
        ('keyset-no-keys','no-keys',approval,no('invalid_keyset','schema')),
        ('keyset-extra-field','extra-field',approval,no('invalid_keyset','schema')),
        ('keyset-unknown-status','unknown-status',approval,no('invalid_keyset','schema')),
        ('keyset-other-algorithm','other-algorithm',approval,no('unsupported_algorithm')),
        ('keyset-short-key','short-key',approval,no('invalid_keyset','key_encoding')),
        ('keyset-key-with-white-space','key-with-white-space',approval,no('invalid_keyset','key_encoding')),
        ('keyset-key-id-of-another-key','key-id-of-another-key',approval,no('invalid_keyset','key_id')),
        ('keyset-duplicate-key','duplicate-key',approval,no('invalid_keyset','duplicate_key')),
        ('keyset-before-record','format-major-2',[approval],no('unsupported_keyset_format')),
    ]
    return {'format_version':1,
            'note':'Test data. The seeds are public. Never use these keys outside the vectors.',
            'keys':{name:{'seed':b64(SEEDS[name]),'public_key':b64(PUBLIC[name]),'key_id':KEY_ID[name]} for name in sorted(SEEDS)},
            'records':{'decision':DECISION,'event':EVENT},
            'key_sets':sets,
            'cases':[{'name':n,'key_set':k,'signed':s,'expect':e} for n,k,s,e in cases]}

def text():return json.dumps(build(),indent=1,sort_keys=True)+'\n'

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--write',action='store_true');a=p.parse_args()
    if a.write:OUT.write_text(text())
    else:sys.stdout.write(text())
    return 0

if __name__=='__main__':raise SystemExit(main())
