#!/usr/bin/env python3
"""Regression checks: each quickstart failure path writes summary.json with a named reason."""
import contextlib
import copy
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0,str(Path(__file__).resolve().parent))
import demo
if not os.environ.get('QUICKSTART_TEST_WORK'):sys.exit('set QUICKSTART_TEST_WORK to a work directory outside the checkout')
WORK=Path(os.environ['QUICKSTART_TEST_WORK']);WORK.mkdir(parents=True,exist_ok=True)
FIXTURE=json.loads(demo.FIXTURE.read_text());EXPECTED=FIXTURE['expected']
BUILD={'implementation_ref':'sha256:'+'0'*64,'build':{'Deps':[{'Path':'cel.dev/cel-go','Version':'v0.32.0'}]}}
ANSWER_SNAPSHOT='sha256:'+'a'*64;CORRECTION_SNAPSHOT='sha256:'+'c'*64
def step(name,result):return {'name':name,'response':{'data':{'result':result}}}
def decision(value,snapshot):return {'actual':{'value':value},'receipt_ref':'sha256:'+'r'*64,'receipt':{'selection':{'snapshot_ref':snapshot}}}
# Invented scenario results. The values come from the fixture's expected field, not from a demo run.
PASSED={'status':'passed','pins':{'correction_event':'sha256:'+'e'*64,'correction_snapshot':CORRECTION_SNAPSHOT},
        'trace':[step('november-before-correction',decision(EXPECTED['future_november_reviewed'],ANSWER_SNAPSHOT)),
                 step('correction-source',{'source_ref':'sha256:'+'s'*64}),
                 step('pinned-replay',decision(EXPECTED['pinned_replay'],ANSWER_SNAPSHOT)),
                 step('later-restatement',decision(EXPECTED['later_restatement'],CORRECTION_SNAPSHOT))]}

class QuickstartFailures(unittest.TestCase):
 def conclude(self,results,code=0,stdout='',stderr=''):
  """Write an invented scenario result, then run the demo's summary and report path."""
  w=Path(tempfile.mkdtemp(dir=WORK));(w/'loop').mkdir()
  if results is not None:(w/'loop/scenario-results.json').write_text(results if isinstance(results,str) else json.dumps(results))
  loop=subprocess.CompletedProcess(['scenario.py'],code,stdout,stderr);out=io.StringIO()
  with contextlib.redirect_stdout(out):
   summary=demo.summarize(w,FIXTURE,BUILD,loop);demo.write_summary(w,summary);demo.report(summary)
  return json.loads((w/'summary.json').read_text()),out.getvalue()
 def test_control_the_invented_results_pass(self):
  summary,printed=self.conclude(PASSED)
  self.assertEqual(summary['status'],'passed');self.assertEqual(summary['failed_checks'],[]);self.assertIn('Result: passed',printed)
 def test_loop_failure_keeps_stdout_and_stderr(self):
  summary,printed=self.conclude({'status':'failed','error':'store rejected the correction'},2,'partial trace line\n','Traceback: scenario stopped\n')
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['loop_error'],'store rejected the correction')
  self.assertEqual(summary['loop_output'],{'exit_code':2,'stdout':'partial trace line\n','stderr':'Traceback: scenario stopped\n'})
  self.assertIn('loop_output',printed);self.assertIn('Result: failed',printed)
 def test_loop_failure_without_results_keeps_output(self):
  summary,printed=self.conclude(None,1,'','scenario crashed\n')
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['loop_error'],'scenario crashed')
  self.assertEqual(summary['loop_output']['stderr'],'scenario crashed\n');self.assertIn('scenario-results.json does not exist',summary['errors'][0])
 def test_truncated_results_still_write_the_summary(self):
  summary,printed=self.conclude(json.dumps(PASSED)[:40],1,'','killed\n')
  self.assertEqual(summary['status'],'failed');self.assertIn('scenario-results.json is not valid JSON',summary['errors'][0])
  self.assertEqual(summary['loop_output']['stderr'],'killed\n');self.assertIn('Error: scenario-results.json is not valid JSON',printed)
 def test_invalid_results_with_a_zero_exit_fail(self):
  summary,printed=self.conclude('[]')
  self.assertEqual(summary['status'],'failed');self.assertIn('not valid JSON: the top level is not an object',summary['errors'][0])
  self.assertNotIn('The loop failed',printed);self.assertIn('Error: scenario-results.json is not valid JSON',printed)
 def test_value_mismatch_names_the_comparison(self):
  results=copy.deepcopy(PASSED);results['trace'][2]['response']['data']['result']['actual']['value']=not EXPECTED['pinned_replay']
  summary,printed=self.conclude(results)
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['failed_checks'],['replay-value'])
  self.assertIn('Failed comparison: replay-value',printed)
 def test_snapshot_mismatch_names_the_comparison(self):
  results=copy.deepcopy(PASSED);results['trace'][3]['response']['data']['result']['receipt']['selection']['snapshot_ref']=ANSWER_SNAPSHOT
  summary,printed=self.conclude(results)
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['failed_checks'],['restatement-uses-correction-snapshot'])
  self.assertIn('Failed comparison: restatement-uses-correction-snapshot',printed)
 def test_missing_step_is_a_named_error(self):
  results=copy.deepcopy(PASSED);results['trace']=[x for x in results['trace'] if x['name']!='pinned-replay']
  summary,printed=self.conclude(results)
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['errors'],['step pinned-replay is not in the trace'])
  self.assertIn('Error: step pinned-replay is not in the trace',printed)
 def test_step_without_a_result_is_a_named_error(self):
  results=copy.deepcopy(PASSED);del results['trace'][0]['response']['data']
  summary,printed=self.conclude(results)
  self.assertEqual(summary['status'],'failed');self.assertEqual(summary['errors'],['step november-before-correction has no response.data.result'])
 def test_missing_field_is_a_named_error(self):
  results=copy.deepcopy(PASSED);del results['pins']['correction_snapshot']
  summary,printed=self.conclude(results)
  self.assertEqual(summary['status'],'failed');self.assertIn("KeyError 'correction_snapshot'",summary['errors'][0])
 def test_failed_step_writes_the_summary(self):
  w=Path(tempfile.mkdtemp(dir=WORK));err=io.StringIO();out=io.StringIO()
  with contextlib.redirect_stderr(err),contextlib.redirect_stdout(out),self.assertRaises(SystemExit) as caught:demo.step_failed(w,'build-host',1,'out\n','go: no network\n')
  self.assertEqual(caught.exception.code,1);self.assertIn('build-host failed (exit 1)',err.getvalue())
  self.assertIn('Result: failed. build-host failed. Details: %s'%(w/'summary.json'),out.getvalue())
  summary=json.loads((w/'summary.json').read_text())
  self.assertEqual((summary['status'],summary['failed_step'],summary['stdout'],summary['stderr']),('failed','build-host','out\n','go: no network\n'))

class QuickstartToolCheck(unittest.TestCase):
 def go(self,code,stdout,stderr=''):return demo.go_problem(subprocess.CompletedProcess(['go','version'],code,stdout,stderr))
 def test_control_the_pinned_go_passes(self):self.assertIsNone(self.go(0,'go version go1.27.2 darwin/arm64\n'))
 def test_a_later_go_passes(self):self.assertIsNone(self.go(0,'go version go1.28 linux/amd64\n'))
 def test_an_older_go_names_the_version(self):
  problem=self.go(0,'go version go1.26.9 linux/amd64\n');self.assertIn('Go 1.27.2 or later is necessary; found go version go1.26.9',problem)
 def test_a_failed_go_keeps_stderr(self):
  problem=self.go(1,'','dyld: Library not loaded\n');self.assertIn('failed (exit 1): dyld: Library not loaded',problem)
 def test_a_devel_go_passes(self):self.assertIsNone(self.go(0,'go version devel go1.28-abc123 linux/amd64\n'))

class QuickstartReport(unittest.TestCase):
 def report(self,summary):
  out=io.StringIO()
  with contextlib.redirect_stdout(out):demo.report(summary)
  return out.getvalue()
 def test_failure_before_the_restatement_keeps_the_build(self):
  w=Path(tempfile.mkdtemp(dir=WORK));(w/'loop').mkdir();loop=subprocess.CompletedProcess(['scenario.py'],2,'','stopped\n')
  summary=demo.summarize(w,FIXTURE,BUILD,loop);printed=self.report(summary)
  self.assertNotIn('restatement',summary);self.assertIn('Build:   %s'%demo.short(BUILD['implementation_ref']),printed)
 def test_an_archived_record_is_named(self):
  w=Path(tempfile.mkdtemp(dir=WORK));(w/'loop').mkdir();(w/'loop/scenario-results.json').write_text(json.dumps(PASSED))
  fixture=copy.deepcopy(FIXTURE);fixture['original_input']={'state':'reviewed','archived':True}
  printed=self.report(demo.summarize(w,fixture,BUILD,subprocess.CompletedProcess(['scenario.py'],0,'','')))
  self.assertIn('Is a "reviewed", archived record',printed);self.assertIn('Input:   {"state":"reviewed","archived":true}',printed)

if __name__=='__main__':
 class Results(unittest.TextTestResult):
  def __init__(self,*a,**kw):super().__init__(*a,**kw);self.passed=[]
  def addSuccess(self,test):self.passed.append(test.id());super().addSuccess(test)
 r=unittest.TextTestRunner(verbosity=2,resultclass=Results).run(unittest.defaultTestLoader.loadTestsFromNames(['__main__.QuickstartFailures','__main__.QuickstartToolCheck','__main__.QuickstartReport']))
 record={'tests_run':r.testsRun,'passed':r.passed,'failures':[{'test':t.id(),'traceback':s} for t,s in r.failures],'errors':[{'test':t.id(),'traceback':s} for t,s in r.errors],'successful':r.wasSuccessful()}
 (WORK/'test-results.json').write_text(json.dumps(record,indent=2)+'\n');raise SystemExit(not r.wasSuccessful())
