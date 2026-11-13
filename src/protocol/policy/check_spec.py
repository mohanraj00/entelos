#!/usr/bin/env python3
"""Check the proposed policy verdict vectors without a Host.

The checker applies the rules of POLICY.md to each case. A pass shows that
the vectors, the schemas and the rules agree. It does not show that a Host agrees.
It does not run CEL: each case states the value of the policy call.
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

VERDICT=json.loads((HERE/'schema'/'verdict.schema.json').read_text())
POLICY=json.loads((HERE/'schema'/'policy.schema.json').read_text())
SPEC=(HERE/'POLICY.md').read_text()
OUTCOMES=('accept','refuse','route')
EVIDENCE_INPUTS=('evidence_human_reviewed','evidence_model_reviewed','evidence_all_automatic','evidence_corrected')

def section(title):
    return SPEC.split('\n## '+title+'\n',1)[1].split('\n## ',1)[0]

def table_reasons():
    """The route reasons in the rules table of POLICY.md."""
    return set(re.findall(r'^\| [0-9]+ \|.*\| `route`, reason `([a-z_]+)` \|$',section('Rules'),re.M))

def table_inputs():
    """The Host input names in the inputs table of POLICY.md."""
    return set(re.findall(r'^\| `([a-z_]+)` \| (?:bool|string) \|',section('Policy inputs'),re.M))

def table_codes(title):
    return set(re.findall(r'^\| [0-9]+ \|.*\| `([a-z_]+)` \|$',section(title),re.M))

def content_ref(value):
    """The inputs_ref: SHA-256 of the media type, a NUL byte and the canonical record bytes."""
    raw=(json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False)+'\n').encode()
    return 'sha256:'+hashlib.sha256(b'application/json\0'+raw).hexdigest()

def selected(nonce,rate):
    """The audit draw is the first 4 bytes of the nonce, big-endian. It selects if draw / 2^32 < rate / 10^6."""
    return int(nonce[:8],16)*1000000<rate*2**32

def host_inputs(sources):
    """The values that the Host builds. A missing source gives no value for its inputs."""
    out={}
    if sources['evidence'] is not None:out.update({k:sources['evidence'][k] for k in EVIDENCE_INPUTS})
    report=sources['report']
    if report is not None:
        out['report_passed']=report['passed']
        if report['change_class'] is not None:out['change_class']=report['change_class']
    return out

def requirement(ctx,head):
    for entry in ctx['requirements']:
        if entry['head']==head:return entry['requirement']
    return 'human'

def changes_policy(ctx,head):
    return head['kind']=='cel-bundle' and head['function'] in ctx['policy_functions']

def map_call(call):
    """Rules 8 and 9: the outcome and the reason that a call value gives."""
    if 'error' in call:return 'route','policy_error'
    if call['output'] not in OUTCOMES:return 'route','invalid_output'
    return call['output'],'policy_output'

def evaluate(case):
    """Apply the rules of POLICY.md in their order. Return the verdict record and whether the Host calls the policy."""
    ctx,item,policy=case,case['item'],case['policy']
    def record(outcome,reason,receipt=None,audit=None):
        return {'format':'entelos.policy/1','kind':'policy','outcome':outcome,'reason':reason,'date':ctx['date'],
                'item':{k:item[k] for k in ('snapshot_ref','report_ref','candidate')},'receipt':receipt,'audit':audit}
    if changes_policy(ctx,item['head']):return record('route','policy_change'),False
    if requirement(ctx,item['head'])!='policy':return record('route','human_required'),False
    if policy is None:return record('route','no_policy'),False
    if policy.get('suspended') is True:return record('route','policy_suspended'),False
    try:validate(POLICY,policy,'policy',POLICY['$defs'])
    except SchemaError:return record('route','invalid_policy'),False
    if policy['function'] not in ctx['policy_functions']:return record('route','invalid_policy'),False
    known=table_inputs()
    if any(n not in known for n in policy['inputs']):return record('route','unknown_field'),False
    values=host_inputs(case['sources'])
    if any(n not in values for n in policy['inputs']):return record('route','missing_input'),False
    if 'call' not in case:raise KeyError(case['name']+': the rules reach the call, and the case has no call')
    inputs={n:values[n] for n in policy['inputs']}
    call=case['call']
    receipt={'policy_function':policy['function'],'activation_ref':policy['activation_ref'],'policy_snapshot_ref':policy['snapshot_ref'],
             'bundle_ref':policy['bundle_ref'],'engine_build':policy['engine_build'],'inputs_ref':content_ref(inputs),
             'output':call.get('output'),'error':call.get('error')}
    outcome,reason=map_call(call)
    audit={'rate_ppm':policy['audit_rate_ppm'],'nonce':case['nonce'],'selected':selected(case['nonce'],policy['audit_rate_ppm'])} if outcome=='accept' else None
    return record(outcome,reason,receipt,audit),True

def replay(verdict,case):
    """Apply the replay rules of POLICY.md to a stored verdict."""
    r=verdict['receipt']
    if r is None:return {'replay':'refused','code':'no_receipt'}
    if content_ref(case['inputs'])!=r['inputs_ref']:return {'replay':'refused','code':'input_mismatch'}
    store=case['store_policy']
    if store is None or store!={'snapshot_ref':r['policy_snapshot_ref'],'bundle_ref':r['bundle_ref']}:return {'replay':'refused','code':'artifact_unavailable'}
    if case['build']!=r['engine_build']:return {'replay':'refused','code':'implementation_unavailable'}
    call=case['call'];outcome,_=map_call(call)
    audit=verdict['audit']
    same=(outcome==verdict['outcome'] and call.get('output')==r['output'] and call.get('error')==r['error']
          and (audit is None or selected(audit['nonce'],audit['rate_ppm'])==audit['selected']))
    return {'replay':'same'} if same else {'replay':'differs','outcome':outcome}

def satisfy(case):
    """Apply the verdict requirement rules of POLICY.md."""
    need='human' if case['changes_policy'] else case['requirement']
    verdicts=[v for v in case['verdicts'] if v['kind']=='human' or need=='policy']
    if any(v.get('disposition')=='reject' or v.get('outcome')=='refuse' for v in verdicts):return 'rejected'
    if any(v.get('disposition')=='accept' or v.get('outcome')=='accept' for v in verdicts):return 'satisfied'
    return 'awaiting_human'

def run(doc):
    errors=[];names=set();reasons_seen=set();outcomes_seen=set();replay_seen=set();results_seen=set()
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']+doc['replays']+doc['requirements']:
        if case['name'] in names:errors.append(case['name']+': duplicate case name')
        names.add(case['name'])
    stored={}
    for raw in doc['cases']:
        case=dict(copy.deepcopy(doc['defaults']),**raw);name,expect=case['name'],case['expect']
        try:validate(VERDICT,expect,name+'.expect',VERDICT['$defs'])
        except SchemaError as e:errors.append(e.message)
        try:got,called=evaluate(case)
        except KeyError as e:errors.append(str(e));continue
        if not called and 'call' in case:errors.append(name+': the rules route before the call, so the case must have no call')
        if got!=expect:errors.append(name+': expected '+json.dumps(expect,sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        stored[name]=expect;reasons_seen.add(expect['reason']);outcomes_seen.add(expect['outcome'])
        if expect['audit'] is not None:outcomes_seen.add('audit_selected' if expect['audit']['selected'] else 'audit_not_selected')
    for case in doc['replays']:
        if case['of'] not in stored:errors.append(case['name']+': no case '+case['of']);continue
        try:got=replay(stored[case['of']],case)
        except KeyError:errors.append(case['name']+': the replay reaches the call, and the case has no call');continue
        if got!=case['expect']:errors.append(case['name']+': expected '+json.dumps(case['expect'],sort_keys=True)+', got '+json.dumps(got,sort_keys=True))
        replay_seen.add(case['expect'].get('code',case['expect']['replay']))
    for case in doc['requirements']:
        if satisfy(case)!=case['expect']:errors.append(case['name']+': expected '+case['expect'])
        results_seen.add(case['expect'])
    reasons=table_reasons()|{'policy_output'}
    errors+=['reason without a case: '+r for r in sorted(reasons-reasons_seen)]
    errors+=['reason not in POLICY.md: '+r for r in sorted(reasons_seen-reasons)]
    errors+=['outcome without a case: '+o for o in sorted(set(OUTCOMES+('audit_selected','audit_not_selected'))-outcomes_seen)]
    errors+=['replay result without a case: '+c for c in sorted((table_codes('Replay')|{'same','differs'})-replay_seen)]
    errors+=['requirement result without a case: '+c for c in sorted({'satisfied','rejected','awaiting_human'}-results_seen)]
    if not any(c['requirement']=='human' and c['verdicts']==[{'kind':'policy','outcome':'accept'}] and c['expect']=='awaiting_human' for c in doc['requirements']):
        errors.append('no case shows that a policy accept does not satisfy a human requirement')
    return errors

def self_test(doc):
    """Change a copy of the vectors. The checker must report each change."""
    def case(d,n):return next(c for c in d['cases']+d['replays']+d['requirements'] if c['name']==n)
    def change_digest(d):r=case(d,'accept-typo-passed')['expect']['receipt'];r['inputs_ref']=r['inputs_ref'][:-1]+'0'
    def flip_audit(d):case(d,'accept-audit-selected')['expect']['audit']['selected']=False
    def accept_suspended(d):e=case(d,'policy-suspended');e['call']={'output':'accept'}
    def wrong_reason(d):case(d,'unknown-field')['expect']['reason']='missing_input'
    def drop_reason(d):d['cases']=[c for c in d['cases'] if c['expect']['reason']!='policy_change']
    def human_by_policy(d):case(d,'policy-accept-on-human-head')['expect']='satisfied'
    def policy_change_by_policy(d):case(d,'policy-accept-on-policy-change')['expect']='satisfied'
    def replay_same(d):case(d,'replay-differs')['expect']={'replay':'same'}
    def replay_build(d):case(d,'replay-other-build')['build']='sha256:'+'d'*64
    def refuse_audit(d):case(d,'refuse-rule-failed')['expect']['audit']={'rate_ppm':100000,'nonce':'0'*64,'selected':True}
    def unknown_record_field(d):case(d,'route-rule-passed')['expect']['approved_by']='policy'
    missed=[]
    for f in (change_digest,flip_audit,accept_suspended,wrong_reason,drop_reason,human_by_policy,policy_change_by_policy,
              replay_same,replay_build,refuse_audit,unknown_record_field):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/'policy-cases.json').read_text())
    errors=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'cases':len(doc['cases']),'replays':len(doc['replays']),'requirements':len(doc['requirements']),'errors':errors,
                      'scope':'Proposed policy verdict vectors: rule order, fail-closed routes, the receipt, the audit draw, replay and the verdict requirement. No Host and no CEL engine ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
