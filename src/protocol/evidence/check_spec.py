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
INPUTS=('evidence_human_reviewed','evidence_model_reviewed','evidence_all_automatic','evidence_corrected')
VALUED=('admitted','confirmed','chosen','corrected')

def table_terms(text):
    """The rule names and the codes in the rules table of EVIDENCE.md."""
    section=text.split('\n## Rules\n',1)[1].split('\n## ',1)[0]
    return set(re.findall(r'rule `([a-z_]+)`',section)),set(re.findall(r'\| `([a-z_]+)`',section))

def expand(value):
    """Replace each {"$repeat": {"count", "fact"}} with count copies. {i} in fact_id is the index.
    {"$repeat": {"count", "item"}} gives count plain copies of the item.
    {"$text": {"count", "char"}} gives a string of count copies of char."""
    if isinstance(value,dict):
        if set(value)=={'$text'}:return value['$text']['char']*value['$text']['count']
        if set(value)=={'$repeat'}:
            r=value['$repeat']
            if 'item' in r:return [expand(r['item']) for _ in range(r['count'])]
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
    def review(f):return f.get('review')
    def status(f):return f['review']['status'] if 'review' in f else None
    if len(facts)>MAX_FACTS:return bad('too_many_facts')
    if len({f['fact_id'] for f in facts})!=len(facts):return bad('duplicate_fact')
    def part_ok(p):
        if p['start'] is None and p['end'] is None:return True
        return p['start'] is not None and p['end'] is not None and p['end']>p['start']
    if any(s['end']<=s['start'] for f in facts for s in f['spans']):return bad('span_order')
    if not all(part_ok(p) for f in facts for p in f['data'].get('parts',[])):return bad('span_order')
    if any(f['spans'] and f['text_digest'] is None for f in facts):return bad('span_without_text')
    for f in facts:
        for s in f['spans']:
            if 'box' in s and ('page' not in s or not (s['box']['x0']<s['box']['x1'] and s['box']['y0']<s['box']['y1'])):return bad('locator')
    if any(status(f)=='corrected' and 'correction_receipt_digest' not in f['review'] for f in facts):return bad('correction_receipt_required')
    if any(review(f) and status(f)!='corrected' and 'correction_receipt_digest' in f['review'] for f in facts):return bad('correction_receipt_forbidden')
    if any(status(f)=='admitted' and f['outcome']!='admitted' for f in facts):return bad('review_status_outcome')
    if any(f['outcome']=='needs_verification' and not review(f) for f in facts):return bad('unreviewed_fact')
    if any((f['outcome']=='rejected' and not review(f)) or status(f) in ('rejected','not_in_document') for f in facts):return bad('rejected_fact')
    if any(status(f) in ('open','disputed') for f in facts):return bad('open_review')
    kinds={a['reviewer_kind'] for f in facts if review(f) for a in f['review']['answers']}
    return {'valid':True,'inputs':{'evidence_human_reviewed':'human' in kinds,'evidence_model_reviewed':'model' in kinds,
                                   'evidence_all_automatic':not any(review(f) for f in facts),
                                   'evidence_corrected':any(status(f)=='corrected' for f in facts)}}

def run(doc):
    errors=[];names=set();rules_seen=set();codes_seen=set()
    rules,codes=table_terms(SPEC)
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']:
        name=case['name']
        if name in names:errors.append(name+': duplicate case name')
        names.add(name)
        expect=case['expect']
        if expect.get('valid') is True and set(expect.get('inputs',{}))!=set(INPUTS):errors.append(name+': a valid case needs the four inputs')
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
    def flip_input(d):i=case(d,'valid-human-confirmed')['expect']['inputs'];i['evidence_human_reviewed']=not i['evidence_human_reviewed']
    def flip_corrected(d):i=case(d,'valid-corrected')['expect']['inputs'];i['evidence_corrected']=False
    def change_rule(d):case(d,'span-empty')['expect']['rule']='schema'
    def accept_rejected(d):case(d,'rejected-by-review')['evidence']['facts'][0]['review']['status']='confirmed'
    def drop_rule_case(d):d['cases']=[c for c in d['cases'] if c['expect'].get('rule')!='open_review']
    def grow_list(d):case(d,'valid-256-facts')['evidence']['facts']['$repeat']['count']=257
    def major_as_format(d):case(d,'format-major-2')['expect']={'valid':False,'code':'invalid_evidence','rule':'format'}
    def drop_correction_receipt(d):del case(d,'valid-corrected')['evidence']['facts'][0]['review']['correction_receipt_digest']
    def add_unknown_field(d):case(d,'valid-full-data')['evidence']['facts'][0]['data']['proposer']='model-a'
    def long_value(d):case(d,'valid-value-max')['evidence']['facts'][0]['data']['value']['$text']['count']=65537
    def swap_order(d):case(d,'rejected-before-open')['expect']['rule']='open_review'
    missed=[]
    for f in (flip_input,flip_corrected,change_rule,accept_rejected,drop_rule_case,grow_list,major_as_format,
              drop_correction_receipt,add_unknown_field,long_value,swap_order):
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
