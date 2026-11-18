#!/usr/bin/env python3
"""Check the proposed profile promotion vectors without a Host.

The checker applies the rules of PROMOTION.md to each case. It applies the
rules of CONTENT.md and POLICY.md through their checkers. A pass shows that
the vectors, the schemas and the rules agree. It does not show that a Host
agrees. It does not run CEL: each case states the value of the policy call.
"""
import argparse
import copy
from fractions import Fraction
import hashlib
import importlib.util
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

def load(name,path):
    spec=importlib.util.spec_from_file_location(name,path)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    return module

CONTENT=load('content_check_spec',HERE.parent/'content'/'check_spec.py')
POLICY=load('policy_check_spec',HERE.parent/'policy'/'check_spec.py')
SUMMARY=json.loads((HERE/'schema'/'summary.schema.json').read_text())
SPEC=(HERE/'PROMOTION.md').read_text()
ITEMS=('measure','profile')
MEDIA_TYPE='application/json'
VERSION=re.compile(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)')
BOUND=re.compile(r'[01](\.[0-9]+)?')

def section(title):
    return SPEC.split('\n## '+title+'\n',1)[1].split('\n## ',1)[0]

def table_rules():
    """The rule codes in the rules table of PROMOTION.md."""
    return set(re.findall(r'^\| [0-9]+ \|.*\| `([a-z_]+)` \|$',section('Rules'),re.M))

def basis_points(text):
    """The smallest integer that is not less than the bound times 10000, or None for a text that is not a decimal from 0 to 1."""
    if not isinstance(text,str) or not BOUND.fullmatch(text):return None
    value=Fraction(text)*10000
    if value>10000:return None
    return -(-value.numerator//value.denominator)

def judge(ctx,candidate,report):
    """Apply the rules of PROMOTION.md in their order. Return the measure inputs or the first rule that fails."""
    def bad(rule):return {'valid':False,'rule':rule}
    if not isinstance(candidate,dict) or candidate.get('kind')!='content':return bad('not_promotion')
    if not CONTENT.judge(candidate)['valid']:return bad('content')
    doc=json.loads(candidate['manifest'])
    if doc['collection'] not in ctx['promotion_collections']:return bad('not_promotion')
    items={i['item_id']:i for i in doc['items']}
    if sorted(items)!=list(ITEMS) or any(i.get('media_type')!=MEDIA_TYPE for i in items.values()):return bad('items')
    meta=doc.get('metadata',{})
    if meta.get('profile_id')!=doc['partition'] or not VERSION.fullmatch(meta.get('profile_version','')):return bad('metadata')
    summary=None if report is None else report.get('measure')
    if summary is None:return bad('no_summary')
    try:validate(SUMMARY,summary,'summary',SUMMARY['$defs'])
    except SchemaError:return bad('summary_schema')
    if summary['profile_digest']!=items['profile']['digest'] or summary['measure_digest']!=items['measure']['digest']:return bad('digest_binding')
    keys=[(r['field'],r['route']) for r in summary['routes']]
    if len(set(keys))!=len(keys):return bad('duplicate_route')
    return {'valid':True,'inputs':{'measure_gold_digest':summary['gold_digest'],
                                   'measure_max_bound_bp':max(r['bound_bp'] for r in summary['routes'])}}

def read_record(candidate,text):
    """The consumer check: the manifest bytes give the candidate ref of the record, and the manifest names the profile."""
    raw=text.encode('utf-8','surrogatepass')
    if candidate.get('kind')!='content' or 'sha256:'+hashlib.sha256(raw).hexdigest()!=candidate.get('ref'):return {'refused':'candidate_mismatch'}
    doc=json.loads(text);items={i['item_id']:i for i in doc['items']}
    return {'profile_id':doc['metadata']['profile_id'],'profile_version':doc['metadata']['profile_version'],
            'profile_digest':items['profile']['digest'],'measure_digest':items['measure']['digest']}

def inputs_of(doc,case):
    """The candidate and the item report of a case."""
    candidate=case.get('candidate') or {'kind':'content','manifest':doc['manifests'][case['manifest']]}
    report=case.get('report',{'passed':True})
    if report is not None:
        name=case.get('summary')
        report={'passed':report['passed'],'measure':None if name is None else copy.deepcopy(doc['summaries'][name])}
    return candidate,report

def verdict(doc,case):
    """The policy verdict that POLICY.md gives with the measure inputs of this file."""
    ctx=dict(copy.deepcopy(doc['defaults']),**case)
    candidate,report=inputs_of(doc,case)
    found=CONTENT.judge(candidate)
    measure=judge(ctx,candidate,report)
    item=dict(ctx['item'],candidate={'kind':'content','ref':found['ref']},head=found['head'])
    sources={'evidence':None,'report':None if report is None else {'passed':report['passed'],'change_class':None},
             'measure':measure.get('inputs')}
    pcase={k:ctx[k] for k in ('name','date','policy_functions','requirements','policy','nonce')}
    pcase.update(item=item,sources=sources)
    if 'call' in case:pcase['call']=case['call']
    return POLICY.evaluate(pcase)

def run(doc):
    errors=[];names=set();rules_seen=set();outcomes_seen=set();reads_seen=set()
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['checks']+doc['cases']+doc['activations']:
        if case['name'] in names:errors.append(case['name']+': duplicate case name')
        names.add(case['name'])
    for case in doc['bounds']:
        got=basis_points(case['bound'])
        if got!=case['expect']:errors.append('bound '+case['bound']+': expected '+json.dumps(case['expect'])+', got '+json.dumps(got))
    if not any(c['expect'] is None for c in doc['bounds']) or not any(c['expect'] is not None for c in doc['bounds']):
        errors.append('the bounds need a valid and a refused case')
    ctx=doc['defaults']
    for case in doc['checks']:
        got=judge(dict(ctx,**case),*inputs_of(doc,case))
        if got!=case['expect']:errors.append(case['name']+': expected '+json.dumps(case['expect'],sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        rules_seen.add(case['expect'].get('rule','valid'))
    for case in doc['cases']:
        name,expect=case['name'],case['expect']
        try:validate(POLICY.VERDICT,expect,name+'.expect',POLICY.VERDICT['$defs'])
        except SchemaError as e:errors.append(e.message)
        try:got,called=verdict(doc,case)
        except KeyError as e:errors.append(str(e));continue
        if not called and 'call' in case:errors.append(name+': the rules route before the call, so the case must have no call')
        if got!=expect:errors.append(name+': expected '+json.dumps(expect,sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        outcomes_seen.add((expect['outcome'],expect['reason']))
        if 'refusal' in case:
            limit=case['refusal']['limit_bp']
            if expect['outcome']!='refuse':errors.append(name+': a refusal needs the outcome refuse')
            if 'measure_max_bound_bp <= %d '%limit not in doc['synthetic_policy']:errors.append(name+': the synthetic policy has another limit')
            above=[r for r in doc['summaries'][case['summary']]['routes'] if r['bound_bp']>limit]
            if above!=case['refusal']['routes']:errors.append(name+': the refusal routes are not the routes above the limit')
    for case in doc['activations']:
        got=read_record(case['candidate'],doc['manifests'][case['manifest']])
        if got!=case['expect']:errors.append(case['name']+': expected '+json.dumps(case['expect'],sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        reads_seen.add('refused' if 'refused' in got else 'read')
    errors+=['rule without a case: '+r for r in sorted((table_rules()|{'valid'})-rules_seen)]
    errors+=['rule not in PROMOTION.md: '+r for r in sorted(rules_seen-table_rules()-{'valid'})]
    for want in (('accept','policy_output'),('refuse','policy_output'),('route','policy_output'),('route','missing_input')):
        if want not in outcomes_seen:errors.append('no case with the outcome %s and the reason %s'%want)
    if not any('refusal' in c for c in doc['cases']):errors.append('no case gives the reason of a refusal')
    errors+=['record read without a case: '+r for r in sorted({'read','refused'}-reads_seen)]
    return errors

def self_test(doc):
    """Change a copy of the vectors. The checker must report each change."""
    def case(d,n):return next(c for c in d['checks']+d['cases']+d['activations'] if c['name']==n)
    def bound_down(d):d['bounds'][1]['expect']=604
    def bound_accepts_large(d):next(c for c in d['bounds'] if c['bound']=='1.5')['expect']=15000
    def summary_digest(d):d['summaries']['v2.0.0']['profile_digest']='sha256:'+'3'*64
    def no_collections(d):d['defaults']['promotion_collections']=[]
    def drop_rule(d):d['checks']=[c for c in d['checks'] if c['expect'].get('rule')!='duplicate_route']
    def accept_inputs(d):r=case(d,'promotion-accepted')['expect']['receipt'];r['inputs_ref']=r['inputs_ref'][:-1]+'0'
    def accept_above_limit(d):case(d,'promotion-refused')['call']={'output':'accept'}
    def call_without_inputs(d):case(d,'unbound-summary-routes-to-a-person')['call']={'output':'accept'}
    def refusal_limit(d):case(d,'promotion-refused')['refusal']['limit_bp']=700
    def refusal_route(d):case(d,'promotion-refused')['refusal']['routes'][0]['route']='checked'
    def record_digest(d):case(d,'record-names-the-profile')['expect']['profile_digest']='sha256:'+'3'*64
    def record_mismatch(d):case(d,'manifest-of-other-record')['manifest']='v2.0.0'
    def human_head(d):d['defaults']['requirements']=[]
    missed=[]
    for f in (bound_down,bound_accepts_large,summary_digest,no_collections,drop_rule,accept_inputs,accept_above_limit,
              call_without_inputs,refusal_limit,refusal_route,record_digest,record_mismatch,human_head):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/'promotion-cases.json').read_text())
    errors=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'bounds':len(doc['bounds']),'checks':len(doc['checks']),'cases':len(doc['cases']),
                      'activations':len(doc['activations']),'errors':errors,
                      'scope':'Proposed profile promotion vectors: the bound conversion, the promotion rules, the policy verdict with the measure inputs and the consumer read of the record. No Host and no CEL engine ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
