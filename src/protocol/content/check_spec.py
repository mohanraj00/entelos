#!/usr/bin/env python3
"""Check the proposed content proposal vectors without a Host.

The checker applies the rules of CONTENT.md to each case. A pass shows that
the vectors, the schemas and the rules agree. It does not show that a Host agrees.
"""
import argparse
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
sys.path.insert(0,str(HERE.parent))
from test_protocol import SchemaError, validate  # noqa: E402

CANDIDATE=json.loads((HERE/'schema'/'candidate.schema.json').read_text())
MANIFEST=json.loads((HERE/'schema'/'manifest.schema.json').read_text())
SPEC=(HERE/'CONTENT.md').read_text()
FORMAT=re.compile(r'entelos\.content/(0|[1-9][0-9]*)')
NAME=re.compile(r'[A-Za-z0-9._-]{1,128}')
KINDS=('cel-bundle','content')
MAX_BYTES=262144
MAX_ITEMS=1024
MAX_METADATA=64

def table_terms(text):
    """The rule names and the codes in the rules table of CONTENT.md."""
    section=text.split('\n## Rules\n',1)[1].split('\n## ',1)[0]
    rows=re.findall(r'^\| [0-9]+ \|.*\| `([a-z_]+)`(?:, rule `([a-z_]+)`)? \|$',section,re.M)
    return {r for _,r in rows if r},{c for c,_ in rows}

class Duplicate(Exception):
    pass

def pairs(items):
    """Keep the member order. Refuse a member name that occurs two times in one object."""
    names=[k for k,_ in items]
    if len(set(names))!=len(names):raise Duplicate()
    return dict(items)

def no_constant(name):
    raise ValueError('not JSON: '+name)

def parse(text):
    """One JSON value as RFC 8259 defines it, or None."""
    try:return json.loads(text,object_pairs_hook=pairs,parse_constant=no_constant)
    except (ValueError,Duplicate):return None

def unknown(value,schema,path):
    """The path of the first member name that the schema does not define, in text order, or None."""
    if '$ref' in schema:return unknown(value,MANIFEST['$defs'][schema['$ref'][len('#/$defs/'):]],path)
    if isinstance(value,dict) and schema.get('type')=='object' and schema.get('additionalProperties') is False:
        for k,v in value.items():
            where=path+'.'+k if path else k
            if k not in schema['properties']:return where
            found=unknown(v,schema['properties'][k],where)
            if found:return found
    if isinstance(value,list) and 'items' in schema:
        for i,v in enumerate(value):
            found=unknown(v,schema['items'],'%s[%d]'%(path,i))
            if found:return found
    return None

def expand(value):
    """Replace each text marker of CONTENT.md with its text."""
    if isinstance(value,dict):
        if set(value)=={'$pad'}:
            t,n=value['$pad']['text'],value['$pad']['count']
            return t[:-1]+' '*n+t[-1:]
        if set(value)=={'$items'}:
            r=value['$items']
            return r['text'].replace('$ITEMS',','.join(r['item'].replace('{i}','%04d'%i) for i in range(r['count'])))
        return {k:expand(v) for k,v in value.items()}
    if isinstance(value,list):return [expand(v) for v in value]
    return value

def judge(cand):
    """Apply the rules of CONTENT.md in their order. Return the result in the vector shape."""
    def bad(code,rule,**extra):return dict({'valid':False,'code':code,'rule':rule},**extra)
    if not isinstance(cand,dict) or not isinstance(cand.get('kind'),str):return bad('invalid_candidate','kind')
    kind=cand['kind']
    if kind not in KINDS:return {'valid':False,'code':'unsupported_candidate_kind'}
    try:validate(CANDIDATE['$defs'][kind],cand,'candidate',CANDIDATE['$defs'])
    except SchemaError:return bad('invalid_candidate','candidate_schema')
    if kind=='cel-bundle':return {'valid':True,'kind':kind}
    text=cand['manifest'];raw=text.encode('utf-8','surrogatepass')
    if len(raw)>MAX_BYTES:return bad('invalid_content','too_large')
    doc=parse(text)
    if not isinstance(doc,dict):return bad('invalid_content','json')
    if not isinstance(doc.get('format'),str):return bad('invalid_content','format')
    m=FORMAT.fullmatch(doc['format'])
    if not m:return bad('invalid_content','format')
    if m.group(1)!='1':return {'valid':False,'code':'unsupported_content_format'}
    field=unknown(doc,MANIFEST,'')
    if field:return bad('invalid_content','unknown_field',field=field)
    try:validate(MANIFEST,doc,'manifest',MANIFEST['$defs'])
    except SchemaError:return bad('invalid_content','schema')
    ids=[i['item_id'] for i in doc['items']]
    if len(ids)>MAX_ITEMS:return bad('invalid_content','too_many_items')
    if len(set(ids))!=len(ids):return bad('invalid_content','duplicate_item')
    if ids!=sorted(ids):return bad('invalid_content','item_order')
    meta=doc.get('metadata',{})
    if len(meta)>MAX_METADATA:return bad('invalid_content','too_many_metadata')
    if any(not NAME.fullmatch(k) for k in meta):return bad('invalid_content','metadata_name')
    return {'valid':True,'kind':'content','ref':'sha256:'+hashlib.sha256(raw).hexdigest(),
            'head':{'kind':'content','collection':doc['collection'],'partition':doc['partition']}}

def bind(report,snapshot):
    """A report qualifies a snapshot only if the kind and the ref are equal."""
    return 'bound' if (report['kind'],report['ref'])==(snapshot['kind'],snapshot['ref']) else 'stale_qualification'

def run(doc):
    errors=[];names=set();rules_seen=set();codes_seen=set()
    rules,codes=table_terms(SPEC)
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']+doc['bindings']:
        if case['name'] in names:errors.append(case['name']+': duplicate case name')
        names.add(case['name'])
    for case in doc['cases']:
        name,expect=case['name'],case['expect']
        got=judge(expand(case['candidate']))
        if got!=expect:errors.append(name+': expected '+json.dumps(expect,sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        if expect.get('rule'):rules_seen.add(expect['rule'])
        if expect.get('code'):codes_seen.add(expect['code'])
    for case in doc['bindings']:
        for side in ('report','snapshot'):
            try:validate(CANDIDATE['$defs']['candidate_reference'],case[side],case['name']+'.'+side,CANDIDATE['$defs'])
            except SchemaError as e:errors.append(e.message)
        if bind(case['report'],case['snapshot'])!=case['expect']:errors.append(case['name']+': expected '+case['expect'])
    if {c['expect'] for c in doc['bindings']}!={'bound','stale_qualification'}:errors.append('bindings need a bound and a stale_qualification case')
    errors+=['rule without a case: '+r for r in sorted(rules-rules_seen)]
    errors+=['code without a case: '+c for c in sorted(codes-codes_seen)]
    errors+=['rule not in CONTENT.md: '+r for r in sorted(rules_seen-rules)]
    return errors

def self_test(doc):
    """Change a copy of the vectors. The checker must report each change."""
    def case(d,n):return next(c for c in d['cases']+d['bindings'] if c['name']==n)
    def change_ref(d):e=case(d,'valid-three-items')['expect'];e['ref']=e['ref'][:-1]+('0' if e['ref'][-1]!='0' else '1')
    def same_ref_after_space(d):case(d,'valid-one-space-more')['expect']['ref']=case(d,'valid-three-items')['expect']['ref']
    def change_rule(d):case(d,'item-order')['expect']['rule']='duplicate_item'
    def wrong_field(d):case(d,'unknown-item-field')['expect']['field']='items[0].colour'
    def drop_rule_case(d):d['cases']=[c for c in d['cases'] if c['expect'].get('rule')!='metadata_name']
    def grow_text(d):case(d,'valid-max-size')['candidate']['manifest']['$pad']['count']+=1
    def major_as_format(d):case(d,'format-major-2')['expect']={'valid':False,'code':'invalid_content','rule':'format'}
    def bind_changed(d):case(d,'changed-manifest')['expect']='bound'
    missed=[]
    for f in (change_ref,same_ref_after_space,change_rule,wrong_field,drop_rule_case,grow_text,major_as_format,bind_changed):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/'content-cases.json').read_text())
    errors=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'cases':len(doc['cases']),'bindings':len(doc['bindings']),'errors':errors,
                      'scope':'Proposed content proposal vectors: candidate kinds, manifest schema, rule order, refusal codes, refs and report binding. No Host ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
