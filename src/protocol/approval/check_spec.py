#!/usr/bin/env python3
"""Check the proposed approval API vectors without a Host.

The checks are structural. They do not run a Host, so a pass does not show
that an implementation agrees with APPROVAL.md.
"""
import argparse
import copy
from datetime import datetime
import json
import os
from pathlib import Path
import re
import sys
import tempfile

HERE=Path(__file__).resolve().parent
# test_protocol makes a work directory at import when this is not set. The checker writes nothing there.
os.environ.setdefault('PROTOCOL_TEST_WORK',tempfile.gettempdir())
sys.path.insert(0,str(HERE.parent))
from test_protocol import OPERATIONS as V10, SchemaError, validate  # noqa: E402

APPROVAL={p.name[:-len('.schema.json')]:json.loads(p.read_text()) for p in sorted((HERE/'schema').glob('*.schema.json'))}
CREDENTIALS=APPROVAL.pop('credentials')
SAMPLES={'^sha256:[0-9a-f]{64}$':'sha256:'+'0'*64,'^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$':'2026-01-01T00:00:00Z',
         '^[0-9]{4}-[0-9]{2}-[0-9]{2}$':'2026-01-01','^[0-9a-f]{64}$':'0'*64}
STAMP='%Y-%m-%dT%H:%M:%SZ'

def refusals(text):
    """The codes of the refusal table, and the codes that the table marks as new."""
    section=text.split('## Refusal codes',1)[1].split('\n## ',1)[0]
    codes,new=set(),set()
    for line in section.splitlines():
        m=re.match(r'\| (\*\*)?`([a-z_]+)`(\*\*)? \|',line)
        if m:codes.add(m.group(2));(new.add(m.group(2)) if m.group(1) else None)
    return codes,new

def uncovered(text):
    """Codes that the coverage limits name as not in a vector."""
    section=text.split('## Coverage limits',1)[1].split('\n## ',1)[0]
    return set(re.findall(r'`([a-z_]+)`',section))

def sample(schema,root):
    """A value that agrees with a schema. It replaces a marker before the schema check."""
    if '$ref' in schema:return sample(root[schema['$ref'][len('#/$defs/'):]],root)
    if 'const' in schema:return schema['const']
    if 'enum' in schema:return schema['enum'][0]
    kind=schema.get('type')
    if kind=='string':return SAMPLES.get(schema.get('pattern'),'x')
    if kind=='integer':return 0
    if kind=='boolean':return False
    if kind=='array':return []
    if kind=='object':return {k:sample(schema['properties'][k],root) for k in schema.get('required',[])}
    return None

def unmark(value,schema,root):
    """Replace each marker with a sample value of its schema, and keep each $subset field."""
    if '$ref' in schema:return unmark(value,root[schema['$ref'][len('#/$defs/'):]],root)
    if isinstance(value,dict):
        if '$wait' in value:return value['$wait']['seconds']
        if '$subset' in value:
            base=sample(schema,root);base.update({k:unmark(v,schema.get('properties',{}).get(k,{}),root) for k,v in value['$subset'].items()});return base
        if set(value)<={'$capture','$use','$type','$record_bundle'} and value:return sample(schema,root)
        props=schema.get('properties',{})
        return {k:unmark(v,props.get(k,{}),root) for k,v in value.items()}
    if isinstance(value,list):return [unmark(v,schema.get('items',{}),root) for v in value]
    return value

def walk(value):
    yield value
    if isinstance(value,dict):
        for v in value.values():yield from walk(v)
    elif isinstance(value,list):
        for v in value:yield from walk(v)

def check_file(name,doc,codes,new_seen):
    errors=[]
    for key in ('description','protocol_version','context','principals','clock','steps'):
        if key not in doc:errors.append('%s: missing %s'%(name,key))
    if errors:return errors
    if doc['protocol_version']!='1.1':errors.append(name+': protocol_version must be 1.1')
    if {'actor','roles'}&set(doc['context']):errors.append(name+': context must not have actor or roles')
    principals=doc['principals'];actors=[p['actor'] for p in principals.values()]
    if len(set(actors))!=len(actors):errors.append(name+': duplicate principal actor')
    for pname,p in principals.items():
        try:validate(CREDENTIALS['$defs']['file']['properties']['principals']['items'],dict(p,token_sha256='0'*64),name+'.principals.'+pname,CREDENTIALS['$defs'])
        except SchemaError as e:errors.append(e.message)
    clock=datetime.strptime(doc['clock'],STAMP);proposed={};names=set();captures={}
    for step in doc['steps']:
        where=name+':'+step.get('name','?')
        if step.get('name') in names:errors.append(where+': duplicate step name')
        names.add(step.get('name'))
        if step.get('principal')!='$unknown' and step.get('principal') not in principals:errors.append(where+': unknown principal')
        if 'clock' in step:
            later=datetime.strptime(step['clock'],STAMP)
            if later<clock:errors.append(where+': clock goes back')
            clock=later
        request,response=step['request'],step['expect']['response']
        op,version=request['operation'],request['protocol']
        supported=version in ('1.0','1.1')
        if version=='1.1' and op in APPROVAL:root=APPROVAL[op]['$defs']
        elif op in V10:root=V10[op]['$defs']
        elif op in APPROVAL:root=None
        else:errors.append(where+': unknown operation '+op);continue
        if response['protocol']!=(version if supported else '1.1'):errors.append(where+': wrong response version')
        for value in walk(request['body']):
            if isinstance(value,dict) and '$use' in value and value['$use'] not in captures:errors.append(where+': $use before $capture: '+value['$use'])
        if root and response['status']=='ok':
            if not supported:errors.append(where+': success for an unsupported version')
            try:validate(root['request_body'],unmark(request['body'],root['request_body'],root),where+'.body',root)
            except SchemaError as e:errors.append(e.message)
            try:validate(root['result'],unmark(response['result'],root['result'],root),where+'.result',root)
            except SchemaError as e:errors.append(e.message)
        if response['status']!='ok':
            code=response['diagnostic']['code']
            if code!=response['status']:errors.append(where+': status and code differ')
            if code not in codes:errors.append(where+': code not in the refusal table: '+code)
            new_seen.add(code)
            if step['principal']=='$unknown' and response['request_id']!='':errors.append(where+': unauthenticated request_id must be empty')
        for value in walk(response):
            if not isinstance(value,dict):continue
            if '$capture' in value:
                captures.setdefault(value['$capture'],step)
                if op=='propose':proposed[value['$capture']]=clock
            if '$wait' in value:
                since,seconds=value['$wait']['since'],value['$wait']['seconds']
                if since not in proposed:errors.append(where+': $wait names no propose capture: '+since)
                elif (clock-proposed[since]).total_seconds()!=seconds:
                    errors.append('%s: $wait %d differs from the step clocks (%d)'%(where,seconds,(clock-proposed[since]).total_seconds()))
    for parity in doc.get('parity',[]):
        if parity not in names:errors.append(name+': parity names no step: '+parity)
    return errors

def run(docs):
    text=(HERE/'APPROVAL.md').read_text();codes,new=refusals(text);seen=set();errors=[]
    for name,doc in docs.items():errors+=check_file(name,doc,codes,seen)
    for code in sorted(new-seen-uncovered(text)):errors.append('new code without a vector or a coverage limit: '+code)
    for op in APPROVAL:
        if '`%s`'%op not in text:errors.append('APPROVAL.md does not name the operation '+op)
    return errors

def self_test(docs):
    """Each change must give at least one error."""
    def mutate(f):
        d=copy.deepcopy(docs);f(d);return d
    verdicts='approval-verdicts.json'
    def step(d,n):return next(s for s in d[verdicts]['steps'] if s['name']==n)
    cases={
        'wrong-wait':lambda d:step(d,'list-two-days')['expect']['response']['result']['proposals'][0]['wait_seconds']['$wait'].update(seconds=172801),
        'unknown-code':lambda d:step(d,'same-actor-again')['expect']['response'].update(status='no_such_code'),
        'bad-state':lambda d:step(d,'list-two-days')['expect']['response']['result']['proposals'][0].update(state='open'),
        'context-actor':lambda d:d[verdicts]['context'].update(actor='x'),
        'decision-without-time':lambda d:step(d,'accept')['expect']['response']['result']['result']['decision'].pop('decided_at'),
        'clock-back':lambda d:step(d,'accept').update(clock='2026-11-01T00:00:00Z'),
        'version-not-echoed':lambda d:step(d,'accept')['expect']['response'].update(protocol='1.0'),
    }
    missed=[name for name,f in cases.items() if not run(mutate(f))]
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    docs={p.name:json.loads(p.read_text()) for p in sorted((HERE/'vectors').glob('approval-*.json'))}
    errors=run(docs)+self_test(docs)
    print(json.dumps({'passed':not errors,'files':sorted(docs),'steps':sum(len(d['steps']) for d in docs.values()),'errors':errors,
        'scope':'Proposed approval API vectors: structure, schemas, refusal codes and clock arithmetic. No Host ran.'},indent=2))
    return bool(errors)

if __name__=='__main__':raise SystemExit(main())
