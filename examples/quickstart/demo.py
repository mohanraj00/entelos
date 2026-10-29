#!/usr/bin/env python3
"""Build the pinned host, run the record-filter loop and print the answer, correction, replay and restatement."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[2]
LIFE=ROOT/'src/cel-lifecycle'
FIXTURE=LIFE/'fixtures/lifecycle-scenario.json'

class MissingStep(Exception):pass
def find(trace,name):
 for x in trace:
  if isinstance(x,dict) and x.get('name')==name:
   try:return x['response']['data']['result']
   except (KeyError,TypeError):raise MissingStep('step %s has no response.data.result'%name)
 raise MissingStep('step %s is not in the trace'%name)
def write_summary(w,summary):(w/'summary.json').write_text(json.dumps(summary,indent=2,ensure_ascii=False)+'\n')
def step_failed(w,step,code,stdout,stderr):
 # Keep the failed step's output in summary.json and on stderr, then stop.
 write_summary(w,{'kind':'QuickstartSummary','status':'failed','failed_step':step,'exit_code':code,'stdout':stdout,'stderr':stderr,'work':str(w)})
 sys.stderr.write('%s failed (exit %d)\n%s%s'%(step,code,stdout,stderr))
 print('Result: failed. %s failed. Details: %s'%(step,w/'summary.json'));raise SystemExit(1)

def summarize(w,fixture,build,loop):
 """Make the summary from the loop result. Each failure path records its reason."""
 expected=fixture['expected'];deps={x['Path']:x['Version'] for x in build['build']['Deps']};errors=[]
 path=w/'loop/scenario-results.json'
 try:
  results=json.loads(path.read_text())
  if not isinstance(results,dict):raise ValueError('the top level is not an object')
 except FileNotFoundError:results={'status':'failed'};errors.append('%s does not exist'%path.name)
 except ValueError as error:results={'status':'failed'};errors.append('%s is not valid JSON: %s'%(path.name,error))
 if loop.returncode:
  # Keep the scenario's own error output; a nonzero exit is a failure even with a results file.
  results['status']='failed';results['error']=results.get('error') or loop.stderr.strip() or 'scenario.py exited with status %d'%loop.returncode
 summary={'kind':'QuickstartSummary','function':fixture['function'],'implementation_ref':build['implementation_ref'],'cel_go':deps.get('cel.dev/cel-go'),
          'loop_status':results.get('status'),'loop_error':results.get('error'),
          'loop_output':{'exit_code':loop.returncode,'stdout':loop.stdout,'stderr':loop.stderr},'expected':expected,'work':str(w)}
 checks={}
 if results.get('status')=='passed':
  try:
   trace=results['trace'];pins=results['pins']
   answer=find(trace,'november-before-correction');replay=find(trace,'pinned-replay');restated=find(trace,'later-restatement')
   correction=find(trace,'correction-source')
   summary|={'input':fixture['original_input'],'date':'2026-11-15',
    'answer':{'value':answer['actual']['value'],'receipt_ref':answer['receipt_ref'],'snapshot_ref':answer['receipt']['selection']['snapshot_ref']},
    'correction':{'from_state':fixture['amendment']['allowed_state'],'to_state':fixture['correction']['allowed_state'],'source_ref':correction['source_ref'],
                  'event_ref':pins['correction_event'],'snapshot_ref':pins['correction_snapshot']},
    'replay':{'value':replay['actual']['value'],'snapshot_ref':replay['receipt']['selection']['snapshot_ref']},
    'restatement':{'value':restated['actual']['value'],'snapshot_ref':restated['receipt']['selection']['snapshot_ref']}}
   # The expected values come from the fixture. The replay must keep the answer's snapshot; the restatement must use the correction.
   checks={'answer-value':summary['answer']['value'] is expected['future_november_reviewed'],
           'replay-value':summary['replay']['value'] is expected['pinned_replay'],
           'restatement-value':summary['restatement']['value'] is expected['later_restatement'],
           'replay-keeps-answer-snapshot':summary['replay']['snapshot_ref']==summary['answer']['snapshot_ref'],
           'restatement-uses-correction-snapshot':summary['restatement']['snapshot_ref']==pins['correction_snapshot']}
  except MissingStep as error:errors.append(str(error))
  except (KeyError,TypeError) as error:errors.append('the scenario results are incomplete: %s %s'%(type(error).__name__,error))
 summary['errors']=errors;summary['checks']=checks;summary['failed_checks']=[k for k,v in checks.items() if not v]
 summary['matches_fixture']=bool(checks) and not errors and not summary['failed_checks']
 summary['status']='passed' if results.get('status')=='passed' and summary['matches_fixture'] else 'failed'
 return summary

def short(ref):return str(ref)[:19]+'...'
def report(summary):
 expected=summary['expected']
 print()
 print('Record-filter loop for %s'%summary['function'])
 print('  Build identity: %s (cel-go %s)'%(short(summary['implementation_ref']),summary['cel_go']))
 if 'restatement' in summary:
  s=summary;q=json.dumps(s['input'],separators=(',',':'))
  print('  1. Answer:      on %s, %s -> %s (fixture expects %s)'%(s['date'],q,json.dumps(s['answer']['value']),json.dumps(expected['future_november_reviewed'])))
  print('                  receipt %s, snapshot %s'%(short(s['answer']['receipt_ref']),short(s['answer']['snapshot_ref'])))
  print('  2. Correction:  visible state "%s" -> "%s", activated as event %s'%(s['correction']['from_state'],s['correction']['to_state'],short(s['correction']['event_ref'])))
  print('                  new snapshot %s; the earlier receipt stays'%short(s['correction']['snapshot_ref']))
  print('  3. Replay:      the same receipt on its pinned snapshot -> %s (fixture expects %s)'%(json.dumps(s['replay']['value']),json.dumps(expected['pinned_replay'])))
  print('  4. Restatement: the same input under the correction -> %s (fixture expects %s)'%(json.dumps(s['restatement']['value']),json.dumps(expected['later_restatement'])))
 if summary['loop_error']:print('  The loop failed: %s'%summary['loop_error'])
 for error in summary['errors']:print('  Error: %s'%error)
 for name in summary['failed_checks']:print('  Failed comparison: %s'%name)
 if summary['loop_output']['exit_code']:print('  The scenario stdout and stderr are in summary.json (loop_output).')
 print('Result: %s. Details: %s'%(summary['status'],Path(summary['work'])/'summary.json'))

def main():
 p=argparse.ArgumentParser(description=__doc__)
 p.add_argument('--work',type=Path,required=True,help='new directory outside this source tree')
 a=p.parse_args();w=a.work.resolve()
 if w==ROOT or ROOT in w.parents:p.error('work must be outside this source tree')
 if w.exists():p.error('use a fresh work directory; prior runs are preserved')
 for name in ['bin','tmp','cache/build','cache/mod','cache/gopath','cache/telemetry']:(w/name).mkdir(parents=True,exist_ok=True)
 (w/'cache/telemetry/mode').write_text('off\n')
 # The same isolated Go and Git settings as verify.py, so the demo builds the same host identity.
 env={k:v for k,v in os.environ.items() if not k.startswith('GIT_')}|{
  'GIT_CONFIG_NOSYSTEM':'1','GIT_CONFIG_GLOBAL':os.devnull,'GIT_TERMINAL_PROMPT':'0',
  'GOCACHE':str(w/'cache/build'),'GOMODCACHE':str(w/'cache/mod'),'GOPATH':str(w/'cache/gopath'),
  'GOTMPDIR':str(w/'tmp'),'TMPDIR':str(w/'tmp'),'TEST_TELEMETRY_DIR':str(w/'cache/telemetry'),
  'GOENV':'off','GOWORK':'off','GOFLAGS':'','GOTOOLCHAIN':'local','PYTHONDONTWRITEBYTECODE':'1',
  'CEL_LIFECYCLE_SOURCE':str(LIFE)}
 def run(step,args,cwd=ROOT):
  result=subprocess.run([str(x) for x in args],cwd=cwd,env=env,capture_output=True,text=True)
  if result.returncode:step_failed(w,step,result.returncode,result.stdout,result.stderr)
  return result
 # Each run has a new module cache, so each run downloads the pinned Go modules.
 print('Building the pinned host (this downloads the pinned Go modules)...',flush=True)
 binary=w/'bin/cel-handoff'
 run('build-host',['go','build','-mod=readonly','-trimpath','-buildvcs=false','-o',binary,'./cmd/cel-handoff'],ROOT/'src/cel-handoff')
 identity=run('host-identity',[binary,'-build'])
 try:build=json.loads(identity.stdout)
 except ValueError as error:step_failed(w,'host-identity',identity.returncode or 1,identity.stdout,identity.stderr+'the output is not valid JSON: %s\n'%error)
 run('prepare-record',[sys.executable,'-B',ROOT/'src/cel-handoff/tools/prepare.py','--bundle',w/'record-bundle','--binary',binary])
 print('Running the record-filter loop...',flush=True)
 loop=subprocess.run([str(x) for x in [sys.executable,'-B',LIFE/'scenario.py','--work',w/'loop','--runtime',binary,'--bundle',w/'record-bundle','--fixture',FIXTURE]],
                     cwd=ROOT,env=env,capture_output=True,text=True)
 summary=summarize(w,json.loads(FIXTURE.read_text()),build,loop)
 write_summary(w,summary);report(summary)
 return summary['status']!='passed'

if __name__=='__main__':raise SystemExit(main())
