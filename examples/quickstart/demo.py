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
 if result.returncode:
  sys.stderr.write('%s failed (exit %d)\n%s%s'%(step,result.returncode,result.stdout,result.stderr));raise SystemExit(1)
 return result
def find(trace,name):
 return next(x['response']['data']['result'] for x in trace if x['name']==name)

print('Building the pinned host (first run downloads the Go modules)...',flush=True)
binary=w/'bin/cel-handoff'
run('build-host',['go','build','-mod=readonly','-trimpath','-buildvcs=false','-o',binary,'./cmd/cel-handoff'],ROOT/'src/cel-handoff')
build=json.loads(run('host-identity',[binary,'-build']).stdout)
deps={x['Path']:x['Version'] for x in build['build']['Deps']}
run('prepare-record',[sys.executable,'-B',ROOT/'src/cel-handoff/tools/prepare.py','--bundle',w/'record-bundle','--binary',binary])
print('Running the record-filter loop...',flush=True)
loop=subprocess.run([sys.executable,'-B',LIFE/'scenario.py','--work',w/'loop','--runtime',binary,'--bundle',w/'record-bundle','--fixture',FIXTURE],
                    cwd=ROOT,env=env,capture_output=True,text=True)
results=json.loads((w/'loop/scenario-results.json').read_text()) if (w/'loop/scenario-results.json').exists() else {'status':'failed','error':loop.stderr,'trace':[],'pins':{}}
fixture=json.loads(FIXTURE.read_text());expected=fixture['expected']
summary={'kind':'QuickstartSummary','function':fixture['function'],'implementation_ref':build['implementation_ref'],'cel_go':deps.get('cel.dev/cel-go'),
         'loop_status':results['status'],'loop_error':results.get('error'),'expected':expected,'work':str(w)}
if results['status']=='passed':
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
 summary['matches_fixture']=(summary['answer']['value'] is expected['future_november_reviewed'] and summary['replay']['value'] is expected['pinned_replay']
  and summary['restatement']['value'] is expected['later_restatement'] and summary['replay']['snapshot_ref']==summary['answer']['snapshot_ref']
  and summary['restatement']['snapshot_ref']==pins['correction_snapshot'])
else:summary['matches_fixture']=False
summary['status']='passed' if summary['matches_fixture'] else 'failed'
(w/'summary.json').write_text(json.dumps(summary,indent=2,ensure_ascii=False)+'\n')
def short(ref):return ref[:19]+'...'
print()
print('Record-filter loop for %s'%summary['function'])
print('  Build identity: %s (cel-go %s)'%(short(summary['implementation_ref']),summary['cel_go']))
if results['status']=='passed':
 s=summary;q=json.dumps(s['input'],separators=(',',':'))
 print('  1. Answer:      on %s, %s -> %s (fixture expects %s)'%(s['date'],q,json.dumps(s['answer']['value']),json.dumps(expected['future_november_reviewed'])))
 print('                  receipt %s, snapshot %s'%(short(s['answer']['receipt_ref']),short(s['answer']['snapshot_ref'])))
 print('  2. Correction:  visible state "%s" -> "%s", activated as event %s'%(s['correction']['from_state'],s['correction']['to_state'],short(s['correction']['event_ref'])))
 print('                  new snapshot %s; the earlier receipt stays'%short(s['correction']['snapshot_ref']))
 print('  3. Replay:      the same receipt on its pinned snapshot -> %s (fixture expects %s)'%(json.dumps(s['replay']['value']),json.dumps(expected['pinned_replay'])))
 print('  4. Restatement: the same input under the correction -> %s (fixture expects %s)'%(json.dumps(s['restatement']['value']),json.dumps(expected['later_restatement'])))
else:print('  The loop failed: %s'%summary['loop_error'])
print('Result: %s. Details: %s'%(summary['status'],w/'summary.json'))
raise SystemExit(summary['status']!='passed')
