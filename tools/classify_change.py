#!/usr/bin/env python3
"""Classify changed paths as code, security-relevant or docs. CI and cross_review.py share these rules."""
import argparse
from fnmatch import fnmatch
import json
import subprocess
import sys

# A code change runs the full suite. A docs-only change runs the project check only.
CODE=['src/*','examples/*','tests/*','verify.py','.github/workflows/ci.yml']
# A security-relevant change needs a recorded security review before merge.
SECURITY=['.github/*','tools/*','verify.py','src/*.go','*/go.mod','*/go.sum','src/*/cli.py','src/*/client.py',
    'src/*/git_store.py','src/*/cel_runtime.py','src/*/lifecycle.py','examples/*/adapter.py','examples/*/cli.py',
    'SECURITY.md','CONSTITUTION.md','AGENTS.md','CLAUDE.md','GEMINI.md','.agents/*','.claude/*','.codex/*','.gemini/*']
NOT_SECURITY=['*_test.go']

def match(path,patterns):return any(fnmatch(path,x) for x in patterns)
def classify(paths):
    code=[x for x in paths if match(x,CODE)]
    security=[x for x in paths if match(x,SECURITY) and not match(x,NOT_SECURITY)]
    return {'code':bool(code),'security':bool(security),'docs_only':bool(paths) and not code,'code_paths':code,'security_paths':security,'paths':paths}

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('base');p.add_argument('head',nargs='?',default='HEAD')
    p.add_argument('--github-output',help='append code=, security= and docs_only= lines to this file')
    a=p.parse_args()
    # --no-renames lists both ends of a move, so a move out of src/ still counts as a code change.
    out=subprocess.run(['git','diff','--no-renames','--name-only',a.base+'...'+a.head],capture_output=True,text=True)
    if out.returncode:sys.exit(out.stderr.strip() or 'git diff failed')
    result=classify([x for x in out.stdout.splitlines() if x])
    if a.github_output:
        with open(a.github_output,'a') as f:
            for key in ['code','security','docs_only']:f.write(key+'='+str(result[key]).lower()+'\n')
    print(json.dumps(result,indent=2))
