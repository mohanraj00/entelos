#!/usr/bin/env python3
"""Check the supersession vectors without a Host.

The checker has its own model of the selection rules of SUPERSESSION.md. A
pass shows that the vectors and the rules agree. It does not show that a
Host agrees. The Go Host runs the same vectors in
src/lifecycle-core/supersede_test.go.
"""
import argparse
import copy
import json
from pathlib import Path

HERE=Path(__file__).resolve().parent

class Refusal(Exception):
    def __init__(self,code):self.code=code

def live(events,date):
    """The live updates at a date, by the Selection section of SUPERSESSION.md."""
    out=[]
    for e in events:
        if e['from']<=date:
            out=[x for x in out if x not in e['supersedes']]
        if e['from']<=date<e['until']:
            out=[x for x in out if x not in e['targets']]
            if e['kind']=='enable':out.append(e['id'])
    return out

def select(events,date):
    found=live(events,date)
    if not found:raise Refusal('out_of_scope')
    if len(found)>1:raise Refusal('ambiguous_activation')
    return found[0]

def overlaps(a,b):return a['from']<b['until'] and b['from']<a['until']

def add(events,new):
    """The checks of an activation or a suspension, then the append."""
    known={e['id']:e for e in events}
    if len(set(new['targets']))!=len(new['targets']):raise Refusal('invalid_targets')
    for name in new['targets']:
        if name not in known or not overlaps(known[name],new):raise Refusal('invalid_targets')
    seen=set()
    for name in new['supersedes']:
        old=known.get(name)
        if old is None or name in seen or old['kind']!='enable' or not new['from']<old['until']:raise Refusal('invalid_supersession')
        seen.add(name)
    # Each live update in the new interval must have an entry in targets or supersedes.
    dates={new['from']}|{d for e in events for d in (e['from'],e['until']) if new['from']<=d<new['until']}
    for date in sorted(dates):
        for name in live(events,date):
            if name not in new['targets'] and name not in new['supersedes']:raise Refusal('ambiguous_activation')
    if new['kind']=='suspend' and not new['targets']:raise Refusal('invalid_targets')
    events.append(new)

def play(case):
    """Run the steps on the model. Return one result for each step, in the shape of expect."""
    events=[];answers={};results=[]
    for step in case['steps']:
        try:
            if 'activate' in step or 'suspend' in step:
                kind='enable' if 'activate' in step else 'suspend'
                new={'id':step.get('activate',step.get('suspend')),'kind':kind,'from':step['from'],'until':step['until'],
                     'targets':list(step.get('targets',[])),'supersedes':list(step.get('supersedes',[]))}
                if any(e['id']==new['id'] for e in events):raise ValueError(new['id']+': the name is in use')
                add(events,new);results.append(new['id'])
            elif 'resolve' in step:results.append(select(events,step['resolve']))
            elif 'answer' in step:
                answers[step['answer']]=select(events,step['date']);results.append(answers[step['answer']])
            elif 'replay' in step:results.append(answers[step['replay']])
            elif 'record' in step:
                stored=next(e for e in events if e['id']==step['record'])
                results.append(None if stored['supersedes']==step['supersedes'] else {'error':'record names '+json.dumps(stored['supersedes'])})
            else:raise ValueError('unknown step: '+json.dumps(step))
        except Refusal as r:results.append({'error':r.code})
    return results

def expected(step):
    if 'expect' in step:return step['expect']
    if 'record' in step:return None
    return step.get('activate',step.get('suspend'))

def run(doc):
    errors=[];names=set()
    if doc.get('format_version')!=1:errors.append('format_version must be 1')
    for case in doc['cases']:
        name=case['name']
        if name in names:errors.append(name+': duplicate case name')
        names.add(name)
        for i,(step,got) in enumerate(zip(case['steps'],play(case))):
            if got!=expected(step):errors.append('%s step %d: expected %s, got %s'%(name,i,json.dumps(expected(step)),json.dumps(got)))
    chain=next((c for c in doc['cases'] if c['name']=='chain-of-three'),None)
    if chain is None:errors.append('the case chain-of-three is absent')
    else:
        steps=chain['steps']
        if sum(1 for s in steps if 'activate' in s and 'expect' not in s)!=3:errors.append('chain-of-three must activate three updates')
        if sum(1 for s in steps if s.get('supersedes') and 'activate' in s)!=2:errors.append('chain-of-three must have two supersessions')
        if sum(1 for s in steps if 'replay' in s)<2:errors.append('chain-of-three must replay the old answers')
    seen={json.dumps(s['expect'],sort_keys=True) for c in doc['cases'] for s in c['steps'] if isinstance(s.get('expect'),dict)}
    for code in ('out_of_scope','ambiguous_activation','invalid_supersession'):
        if json.dumps({'error':code}) not in seen:errors.append('code without a case: '+code)
    return errors

def self_test(doc):
    """Change a copy of the cases. The checker must report each change."""
    def case(d,n):return next(c for c in d['cases'] if c['name']==n)
    def step(d,n,key,value):return next(s for s in case(d,n)['steps'] if s.get(key)==value)
    def old_after_successor_end(d):step(d,'chain-of-three','resolve','2026-12-15')['expect']='u1'
    def middle_after_third(d):step(d,'chain-of-three','resolve','2026-08-31')['expect']='u2'
    def replay_uses_successor(d):step(d,'chain-of-three','replay','a1')['expect']='u2'
    def february_uses_successor(d):step(d,'chain-of-three','answer','a1-new')['expect']='u2'
    def mask_is_permanent(d):step(d,'mask-is-not-supersession','resolve','2026-06-01')['expect']={'error':'out_of_scope'}
    def suspend_restores(d):step(d,'suspended-successor-does-not-restore','resolve','2026-04-01')['expect']='u1'
    def record_without_name(d):step(d,'chain-of-three','record','u2')['supersedes']=[]
    def late_supersession_accepted(d):
        s=next(s for s in case(d,'refused-supersessions')['steps'] if s.get('from')=='2026-03-01');del s['expect']
    def drop_chain(d):d['cases']=[c for c in d['cases'] if c['name']!='chain-of-three']
    def drop_third(d):case(d,'chain-of-three')['steps']=[s for s in case(d,'chain-of-three')['steps'] if s.get('activate')!='u3' and s.get('record')!='u3' and s.get('expect')!='u3']
    missed=[]
    for f in (old_after_successor_end,middle_after_third,replay_uses_successor,february_uses_successor,mask_is_permanent,
              suspend_restores,record_without_name,late_supersession_accepted,drop_chain,drop_third):
        d=copy.deepcopy(doc);f(d)
        if not run(d):missed.append(f.__name__)
    return ['self-test: no error for '+m for m in missed]

def main():
    p=argparse.ArgumentParser(description=__doc__);p.parse_args()
    doc=json.loads((HERE/'vectors'/'supersession-cases.json').read_text())
    errors=run(doc)+self_test(doc)
    print(json.dumps({'passed':not errors,'cases':len(doc['cases']),'steps':sum(len(c['steps']) for c in doc['cases']),'errors':errors,
                      'scope':'Supersession vectors: selection, refusal codes and the replay binding on a model. No Host ran.'},indent=1))
    return 1 if errors else 0

if __name__=='__main__':raise SystemExit(main())
