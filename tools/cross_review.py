#!/usr/bin/env python3
"""Ask a different harness to review this branch. Claude work goes to Codex and Codex work goes to Claude; agy is the fallback."""
import argparse
from pathlib import Path
import shutil
import subprocess
import sys

sys.path.insert(0,str(Path(__file__).resolve().parent))
from classify_change import classify

ROOT=Path(__file__).resolve().parents[1]
ORDER={'claude':['codex','agy'],'codex':['claude','agy'],'agy':['claude','codex']}
RULES=('Read CONSTITUTION.md and AGENTS.md first. Do not edit files, commit or push. '
    'Report each finding with file:line, severity, a concrete failure scenario and a fix. Say "no findings" if there are none.')
REVIEW='Review the changes in `git diff {base}...HEAD` for correctness, contract breaks, lost failure evidence, missing checks and unclear docs. '
SECURITY=('Do a security review of the changes in `git diff {base}...HEAD`. Examine input framing and parsing, path traversal, command execution, '
    'Git store integrity, workflow permissions and triggers, unpinned actions or dependencies, secrets, and prompt injection in agent instruction files. ')

def command(reviewer,base,prompt):
    if reviewer=='codex':return ['codex','review','--base',base,prompt]
    if reviewer=='claude':return ['claude','-p',prompt,'--permission-mode','plan']
    return ['agy','-p',prompt,'--mode','plan','--sandbox']

def review(builder,base,kind,timeout):
    prompt=(SECURITY if kind=='security' else REVIEW).format(base=base)+RULES;attempts=[]
    for reviewer in ORDER[builder]:
        if not shutil.which(reviewer):attempts.append((reviewer,'not installed'));continue
        try:proc=subprocess.run(command(reviewer,base,prompt),cwd=ROOT,capture_output=True,text=True,timeout=timeout)
        except subprocess.TimeoutExpired:attempts.append((reviewer,'timed out after %ds'%timeout));continue
        if proc.returncode==0 and proc.stdout.strip():return reviewer,proc.stdout.strip(),attempts
        attempts.append((reviewer,'exit %d: %s'%(proc.returncode,(proc.stderr or proc.stdout).strip()[-300:])))
    return None,None,attempts

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--builder',choices=sorted(ORDER),required=True,help='the harness that wrote the change')
    p.add_argument('--base',default='origin/main');p.add_argument('--security',choices=['auto','yes','no'],default='auto',help='auto runs it for security-relevant paths')
    p.add_argument('--timeout',type=int,default=1800)
    a=p.parse_args()
    paths=subprocess.run(['git','diff','--name-only',a.base+'...HEAD'],cwd=ROOT,capture_output=True,text=True,check=True).stdout.split()
    change=classify(paths);kinds=['code']
    if a.security=='yes' or (a.security=='auto' and change['security']):kinds.append('security')
    failed=False
    for kind in kinds:
        reviewer,text,attempts=review(a.builder,a.base,kind,a.timeout)
        print('## %s review\n\nBuilder: %s. Reviewer: %s. Base: %s.\n'%(kind.capitalize(),a.builder,reviewer or 'none',a.base))
        for name,why in attempts:print('- %s was not used: %s'%(name,why))
        if kind=='security':print('\nSecurity-relevant paths: '+(', '.join(change['security_paths']) or 'none (requested)'))
        print('\n'+(text or 'No reviewer was available. Get a manual review.')+'\n');failed|=text is None
    sys.exit(1 if failed else 0)
