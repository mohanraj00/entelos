#!/usr/bin/env python3
"""Check the activation record and suspension record vectors without a Host.

The checker applies the mappings and the rules of ACTIVATION.md to the
vectors. A pass shows that the vectors, the schemas and the rules agree. It
does not show that a Host agrees.
"""
import argparse
import copy
import datetime
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

SCHEMA=json.loads((HERE/'schema'/'activation.schema.json').read_text())
SUSPENSION_SCHEMA=json.loads((HERE/'schema'/'suspension.schema.json').read_text())
SPEC=(HERE/'ACTIVATION.md').read_text()
RECORD_FORMAT='entelos.record/1'


class Duplicate(Exception):
    pass


def pairs(items):
    out={}
    for k,v in items:
        if k in out:raise Duplicate(k)
        out[k]=v
    return out


def no_constant(name):
    raise ValueError(name)


def parse(text):
    """One JSON value as RFC 8259 defines it, or None."""
    try:return json.loads(text,object_pairs_hook=pairs,parse_constant=no_constant)
    except (ValueError,Duplicate):return None


def table_terms(text,heading='## Rules'):
    """The rule names and the codes in one rules table of ACTIVATION.md."""
    section=text.split('\n'+heading+'\n',1)[1].split('\n## ',1)[0]
    rules=set(re.findall(r'rule `([a-z_]+)`',section))
    codes=set(re.findall(r'\| `([a-z_]+)`(?:, rule|\s*\|)',section))
    return rules,codes


def mapping(records,event_ref,request_key):
    """The activation record of ACTIVATION.md for one event, or None."""
    event,snapshot=records['event'],records['snapshot']
    if event.get('kind')!='enable':return None
    out={'format':RECORD_FORMAT,'scope':event['scope'],'function':event['function'],'event_ref':event_ref,'request_key':request_key}
    if 'candidate' in snapshot:
        out.update(candidate=snapshot['candidate'],base=snapshot['base'],collection=snapshot['collection'],partition=snapshot['partition'])
    else:
        base=records.get('base_snapshot')
        out.update(candidate={'kind':'cel-bundle','ref':snapshot['bundle_ref']},base=base['bundle_ref'] if snapshot.get('base_snapshot') else '')
    out.update({'from':event['from'],'until':event['until'],'source_ref':snapshot['source_ref'],'snapshot_ref':event['snapshot_ref'],
                'report_ref':event['report_ref'],'decision_ref':event['decision_ref'],'producer':snapshot['producer'],
                'runner':records['report']['runner'],'approver':records['decision']['actor'],'activator':event['actor'],
                'targets':event['targets'],'supersedes':event.get('supersedes',[]),'applicability':event.get('applicability',{})})
    return out


def suspension_mapping(records,event_ref,request_key):
    """The suspension record of ACTIVATION.md for one event, or None."""
    event=records['event']
    if event.get('kind')!='suspend':return None
    return {'format':RECORD_FORMAT,'scope':event['scope'],'function':event['function'],'event_ref':event_ref,'request_key':request_key,
            'from':event['from'],'until':event['until'],'targets':event['targets'],'actor':event['actor']}


def unknown(value,schema,path,defs=None):
    """The path of the first member name that the schema does not define, in text order, or None."""
    defs=SCHEMA['$defs'] if defs is None else defs
    if '$ref' in schema:return unknown(value,defs[schema['$ref'][len('#/$defs/'):]],path,defs)
    if isinstance(value,dict) and schema.get('type')=='object' and schema.get('additionalProperties') is False:
        for k,v in value.items():
            where=path+'.'+k if path else k
            if k not in schema['properties']:return where
            found=unknown(v,schema['properties'][k],where,defs)
            if found:return found
    return None


def real_date(text):
    try:return datetime.date.fromisoformat(text).isoformat()==text
    except ValueError:return False


def judge(record_kind,text):
    """Apply the rules of ACTIVATION.md in their order. Return the result in the vector shape."""
    def bad(rule,**extra):return dict({'valid':False,'code':'invalid_activation','rule':rule},**extra)
    if record_kind!='activation':return {'valid':False,'code':'unexpected_record_kind'}
    doc=parse(text)
    if not isinstance(doc,dict):return bad('json')
    if doc.get('format')!=RECORD_FORMAT:return {'valid':False,'code':'unsupported_record_format'}
    field=unknown(doc,SCHEMA,'')
    if field:return bad('unknown_field',field=field)
    try:validate(SCHEMA,doc,'activation',SCHEMA['$defs'])
    except SchemaError:return bad('schema')
    content=doc['candidate']['kind']=='content'
    if content!=('collection' in doc) or content!=('partition' in doc):return bad('candidate_fields')
    if content and doc['function']!='content:%s/%s'%(doc['collection'],doc['partition']):return bad('subject')
    if not (real_date(doc['from']) and real_date(doc['until']) and doc['from']<doc['until']):return bad('interval')
    return {'valid':True}


def judge_suspension(record_kind,text):
    """Apply the suspension rules of ACTIVATION.md in their order. Return the result in the vector shape."""
    def bad(rule,**extra):return dict({'valid':False,'code':'invalid_suspension','rule':rule},**extra)
    if record_kind!='suspension':return {'valid':False,'code':'unexpected_record_kind'}
    doc=parse(text)
    if not isinstance(doc,dict):return bad('json')
    if doc.get('format')!=RECORD_FORMAT:return {'valid':False,'code':'unsupported_record_format'}
    field=unknown(doc,SUSPENSION_SCHEMA,'',SUSPENSION_SCHEMA['$defs'])
    if field:return bad('unknown_field',field=field)
    try:validate(SUSPENSION_SCHEMA,doc,'suspension',SUSPENSION_SCHEMA['$defs'])
    except SchemaError:return bad('schema')
    if not (real_date(doc['from']) and real_date(doc['until']) and doc['from']<doc['until']):return bad('interval')
    return {'valid':True}


# One profile for each published record kind: the mapping, the rules and the heading of the rules table.
ACTIVATION={'kind':'activation','mapping':mapping,'judge':judge,'heading':'## Rules','file':'activation-cases.json'}
SUSPENSION={'kind':'suspension','mapping':suspension_mapping,'judge':judge_suspension,'heading':'### Suspension rules','file':'suspension-cases.json'}


def run(doc,profile=ACTIVATION):
    mapping,judge,kind=profile['mapping'],profile['judge'],profile['kind']
    errors=[];names=set();rules_seen=set();codes_seen=set()
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['mappings']+doc['records']:
        if case['name'] in names:errors.append(case['name']+': duplicate case name')
        names.add(case['name'])
    for case in doc['mappings']:
        got=mapping(case['records'],case['event_ref'],case['request_key'])
        want=None if case['expect']=={'none':True} else case['expect']
        if got!=want:errors.append(case['name']+': mapping gives '+json.dumps(got,sort_keys=True))
        if want is not None and judge(kind,json.dumps(want))!={'valid':True}:
            errors.append(case['name']+': the expected record is not valid: '+json.dumps(judge(kind,json.dumps(want))))
    for case in doc['records']:
        got=judge(case['record_kind'],case['record'])
        if got!=case['expect']:errors.append(case['name']+': got '+json.dumps(got,sort_keys=True))
        if not got['valid']:
            codes_seen.add(got['code'])
            if 'rule' in got:rules_seen.add(got['rule'])
    rules,codes=table_terms(SPEC,profile['heading'])
    for r in sorted(rules-rules_seen):errors.append('no record case for rule '+r)
    for c in sorted(codes-codes_seen):errors.append('no record case for code '+c)
    return errors


def self_test(doc):
    """Change a copy of the vectors. The checker must report each change."""
    def case(d,n):return next(c for c in d['mappings']+d['records'] if c['name']==n)
    def change_approver(d):case(d,'content-two-regions')['expect']['approver']='someone-else'
    def drop_applicability(d):case(d,'content-two-regions')['expect']['applicability']={}
    def cel_base_as_snapshot(d):e=case(d,'cel-with-base')['expect'];e['base']=case(d,'cel-with-base')['records']['snapshot']['base_snapshot']
    def suspend_has_record(d):c=case(d,'suspend-has-no-record');c['expect']=mapping(dict(c['records'],event=dict(c['records']['event'],kind='enable')),c['event_ref'],c['request_key'])
    def change_rule(d):case(d,'subject-mismatch')['expect']['rule']='candidate_fields'
    def drop_rule_case(d):d['records']=[c for c in d['records'] if c['expect'].get('rule')!='interval']
    def wrong_field(d):case(d,'unknown-candidate-field')['expect']['field']='digest'
    missed=[]
    tests=(change_approver,drop_applicability,cel_base_as_snapshot,suspend_has_record,change_rule,drop_rule_case,wrong_field)
    for f in tests:
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed],len(tests)


def suspension_self_test(doc):
    """Change a copy of the suspension vectors. The checker must report each change."""
    def case(d,n):return next(c for c in d['mappings']+d['records'] if c['name']==n)
    def run_s(d):return run(d,SUSPENSION)
    def change_actor(d):case(d,'suspend-content-subject')['expect']['actor']='someone-else'
    def copy_reason(d):case(d,'suspend-content-subject')['expect']['reason']='stop'
    def drop_target(d):case(d,'suspend-two-targets-no-key')['expect']['targets'].pop()
    def enable_has_record(d):
        c=case(d,'enable-has-no-record');c['expect']=suspension_mapping({'event':dict(c['records']['event'],kind='suspend')},c['event_ref'],c['request_key'])
    def accept_activation_kind(d):case(d,'record-kind-activation')['expect']={'valid':True}
    def accept_activation_record(d):case(d,'activation-record-as-suspension')['expect']={'valid':True}
    def accept_no_targets(d):case(d,'schema-no-targets')['expect']={'valid':True}
    def change_rule(d):case(d,'interval-reversed')['expect']['rule']='schema'
    def drop_rule_case(d):d['records']=[c for c in d['records'] if c['expect'].get('rule')!='interval']
    def drop_kind_cases(d):d['records']=[c for c in d['records'] if c['expect'].get('code')!='unexpected_record_kind']
    def wrong_field(d):case(d,'unknown-reason-field')['expect']['field']='actor'
    missed=[]
    tests=(change_actor,copy_reason,drop_target,enable_has_record,accept_activation_kind,accept_activation_record,accept_no_targets,change_rule,
           drop_rule_case,drop_kind_cases,wrong_field)
    for f in tests:
        d=copy.deepcopy(doc);f(d)
        if not run_s(d):missed.append(f.__name__)
    return ['self-test: no error for suspension '+m for m in missed],len(tests)


def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/ACTIVATION['file']).read_text())
    suspension=json.loads((HERE/'vectors'/SUSPENSION['file']).read_text())
    missed,changes=self_test(doc)
    suspension_missed,suspension_changes=suspension_self_test(suspension)
    errors=run(doc)+missed+['suspension: '+e for e in run(suspension,SUSPENSION)]+suspension_missed
    print(json.dumps({'passed':not errors,'mappings':len(doc['mappings']),'records':len(doc['records']),'self_test_changes':changes,
                      'suspension_mappings':len(suspension['mappings']),'suspension_records':len(suspension['records']),
                      'suspension_self_test_changes':suspension_changes,'errors':errors,
                      'scope':'Activation record and suspension record vectors: the mapping from stored records, the record schemas, rule order and refusal codes. No Host ran.'},indent=1))
    return 1 if errors else 0


if __name__=='__main__':
    sys.exit(main())
