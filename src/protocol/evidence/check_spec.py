#!/usr/bin/env python3
"""Check the proposed evidence format vectors without a Host.

The checker applies the rules of EVIDENCE.md to each case. A pass shows that
the vectors, the schema and the rules agree. It does not show that a Host agrees.
"""
import argparse
import copy
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

SCHEMA=json.loads((HERE/'schema'/'evidence.schema.json').read_text())
SPEC=(HERE/'EVIDENCE.md').read_text()
FORMAT=re.compile(r'entelos\.evidence/(0|[1-9][0-9]*)')
MAX_FACTS=256
INPUTS=('evidence_human_reviewed','evidence_model_reviewed','evidence_all_automatic')

def table_terms(text):
    """The rule names and the codes in the rules table of EVIDENCE.md."""
    section=text.split('\n## Rules\n',1)[1].split('\n## ',1)[0]
    return set(re.findall(r'rule `([a-z_]+)`',section)),set(re.findall(r'\| `([a-z_]+)`',section))

def expand(value):
    """Replace each {"$repeat": {"count", "fact"}} with count copies. {i} in fact_id is the index."""
    if isinstance(value,dict):
        if set(value)=={'$repeat'}:
            r=value['$repeat']
            return [dict(expand(r['fact']),fact_id=r['fact']['fact_id'].replace('{i}',str(i))) for i in range(r['count'])]
        return {k:expand(v) for k,v in value.items()}
    if isinstance(value,list):return [expand(v) for v in value]
    return value

def judge(doc):
    """Apply the rules of EVIDENCE.md in their order. Return the result in the vector shape."""
    def bad(rule):return {'valid':False,'code':'invalid_evidence','rule':rule}
    if not isinstance(doc,dict) or not isinstance(doc.get('format'),str):return bad('format')
    m=FORMAT.fullmatch(doc['format'])
    if not m:return bad('format')
    if m.group(1)!='1':return {'valid':False,'code':'unsupported_evidence_format'}
    try:validate(SCHEMA,doc,'evidence',SCHEMA['$defs'])
    except SchemaError:return bad('schema')
    facts=doc['facts']
    if len(facts)>MAX_FACTS:return bad('too_many_facts')
    if len({f['fact_id'] for f in facts})!=len(facts):return bad('duplicate_fact')
    if any(f['span']['end']<=f['span']['start'] for f in facts):return bad('span_order')
    if any(f['admission']=='reviewed' and 'review' not in f for f in facts):return bad('review_required')
    if any(f['admission']=='automatic' and 'review' in f for f in facts):return bad('review_forbidden')
    if any(f['review']['outcome']=='rejected' for f in facts if 'review' in f):return bad('rejected_fact')
    kinds={f['review']['reviewer_kind'] for f in facts if 'review' in f}
    return {'valid':True,'inputs':{'evidence_human_reviewed':'human' in kinds,'evidence_model_reviewed':'model' in kinds,
                                   'evidence_all_automatic':all(f['admission']=='automatic' for f in facts)}}

def run(doc):
    errors=[];names=set();rules_seen=set();codes_seen=set()
    rules,codes=table_terms(SPEC)
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']:
        name=case['name']
        if name in names:errors.append(name+': duplicate case name')
        names.add(name)
        expect=case['expect']
        if expect.get('valid') is True and set(expect.get('inputs',{}))!=set(INPUTS):errors.append(name+': a valid case needs the three inputs')
        got=judge(expand(case['evidence']))
        if got!=expect:errors.append(name+': expected '+json.dumps(expect,sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        if expect.get('rule'):rules_seen.add(expect['rule'])
        if expect.get('code'):codes_seen.add(expect['code'])
    errors+=['rule without a case: '+r for r in sorted(rules-rules_seen)]
    errors+=['code without a case: '+c for c in sorted(codes-codes_seen)]
    errors+=['rule not in EVIDENCE.md: '+r for r in sorted(rules_seen-rules)]
    return errors

def self_test(doc):
    """Change a copy of the cases. The checker must report each change."""
    def case(d,n):return next(c for c in d['cases'] if c['name']==n)
    def flip_input(d):i=case(d,'valid-human-review')['expect']['inputs'];i['evidence_human_reviewed']=not i['evidence_human_reviewed']
    def change_rule(d):case(d,'span-empty')['expect']['rule']='schema'
    def accept_rejected(d):case(d,'rejected-fact')['evidence']['facts'][0]['review']['outcome']='confirmed'
    def drop_rule_case(d):d['cases']=[c for c in d['cases'] if c['name']!='review-forbidden']
    def grow_list(d):case(d,'valid-256-facts')['evidence']['facts']['$repeat']['count']=257
    def major_as_format(d):case(d,'format-major-2')['expect']={'valid':False,'code':'invalid_evidence','rule':'format'}
    missed=[]
    for f in (flip_input,change_rule,accept_rejected,drop_rule_case,grow_list,major_as_format):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/'evidence-cases.json').read_text())
    errors=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'cases':len(doc['cases']),'errors':errors,
                      'scope':'Proposed evidence format vectors: schema, rule order, refusal codes and policy inputs. No Host ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
