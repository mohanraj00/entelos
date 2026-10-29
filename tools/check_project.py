#!/usr/bin/env python3
"""Offline shared-harness layout, local-link and regular-source inventory check."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys

ROOT=Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser(description=__doc__);p.add_argument('--write-checksums',action='store_true');a=p.parse_args()
required=['CONSTITUTION.md','AGENTS.md','CLAUDE.md','GEMINI.md','README.md','docs/SUPPORT.md','SECURITY.md',
    'docs/STATUS.md','docs/BACKLOG.md','docs/CONTRACTS.md','.agents/skills/decision-workflow/SKILL.md',
    'docs/REVIEW.md','.github/CODEOWNERS','.github/workflows/ci.yml','.github/workflows/pr-policy.yml','.github/workflows/claude-review.yml','.github/ISSUE_TEMPLATE/bug.yml','.github/ISSUE_TEMPLATE/workflow.yml','.github/ISSUE_TEMPLATE/config.yml']
errors=[];inventory={};links=0
for path in sorted(ROOT.rglob('*')):
    relative=path.relative_to(ROOT)
    if relative.parts[0]=='.git':continue
    name=relative.as_posix()
    if path.is_symlink():
        if name!='.claude/skills' or path.readlink().as_posix()!='../.agents/skills' or path.resolve()!=(ROOT/'.agents/skills').resolve():
            errors.append('unexpected or invalid symlink: '+name)
        continue
    if not path.is_file():continue
    if set(relative.parts)&{'__pycache__','.venv','node_modules'}:errors.append('generated source-side directory: '+name)
    if name!='checksums.json':inventory[name]=hashlib.sha256(path.read_bytes()).hexdigest()
    if path.suffix=='.md':
        for target in re.findall(r'\]\(([^)]+)\)',path.read_text()):
            if target.startswith(('https:','http:','#','mailto:')):continue
            links+=1
            if not (path.parent/target.split('#')[0]).exists():errors.append('broken local link: '+name+' -> '+target)
for name in required:
    if not (ROOT/name).is_file():errors.append('missing: '+name)
alias=ROOT/'.claude/skills'
if not alias.is_symlink():errors.append('missing shared skill link')
claude=(ROOT/'CLAUDE.md').read_text() if (ROOT/'CLAUDE.md').exists() else ''
if '@CONSTITUTION.md' not in claude or '@AGENTS.md' not in claude:errors.append('Claude entry point does not import shared rules')
gemini=(ROOT/'GEMINI.md').read_text() if (ROOT/'GEMINI.md').exists() else ''
if '@CONSTITUTION.md' not in gemini or '@AGENTS.md' not in gemini:errors.append('GEMINI.md does not import shared rules')
# Classification regressions: each path must keep its CI and security-review class.
sys.path.insert(0,str(ROOT/'tools'))
from classify_change import classify
for path,code,security in [('README.md',False,False),('src/cel-lifecycle/lifecycle.py',True,True),
        ('src/cel-lifecycle/git_store.py',True,True),('src/cel-handoff/handoff_test.go',True,False),('src/cel-handoff/go.sum',True,True),
        ('tools/classify_change.py',True,True),('.github/workflows/ci.yml',True,True),('.codex/config.toml',False,True),('GEMINI.md',False,True),
        ('docs/AGENTS.md',False,True),('src/cel-handoff/tools/prepare.py',True,True),('examples/ci-trigger/run.py',True,True),
        ('src/cel-lifecycle/test_lifecycle.py',True,False),('tests/test_export_boundaries.py',True,False),('docs/STATUS.md',False,False),('docs/REVIEW.md',False,True),('src/cel-lifecycle/.claude/settings.json',True,True),
        ('tests/AGENTS.md',True,True),('tests/CLAUDE.md',True,True),('tests/.claude/settings.json',True,True),('tests/go.mod',True,True),('src/cel-handoff/x_test.go',True,False)]:
    got=classify([path])
    if (got['code'],got['security'])!=(code,security):errors.append('classification changed for %s: code=%s security=%s'%(path,got['code'],got['security']))
# Review events default to the PR merge ref, so the Claude review must check out the default branch.
if 'ref: ${{ github.event.repository.default_branch }}' not in (ROOT/'.github/workflows/claude-review.yml').read_text():errors.append('claude-review.yml must check out the default branch')
# PR gate regressions: a fork needs acceptance, and a label counts only with an approval record for the current head and base.
from pr_policy import decide
def event(labels,head='o/r',sha='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',base='main'):return {'action':'edited','repository':{'full_name':'o/r'},'pull_request':{'labels':[{'name':x} for x in labels],'head':{'sha':sha,'repo':{'full_name':head}},'base':{'ref':base}}}
ok={'accepted':{('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','main')},'security-reviewed':{('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','main')}}
for check,case,paths,complete,found,want in [('acceptance',event([]),[],True,{},(True,None)),('acceptance',event([],'f/r'),[],True,{},(False,None)),
        ('acceptance',event(['accepted'],'f/r'),[],True,ok,(True,None)),('acceptance',event(['accepted'],'f/r'),[],True,{},(False,'accepted')),
        ('acceptance',event(['accepted'],'f/r','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'),[],True,ok,(False,'accepted')),('acceptance',event(['accepted'],'f/r',base='dev'),[],True,ok,(False,'accepted')),
        ('security',event([]),['docs/STATUS.md'],True,{},(True,None)),('security',event([]),['docs/STATUS.md'],False,{},(False,None)),
        ('security',event([]),['tools/pr_policy.py'],True,ok,(False,None)),('security',event(['security-reviewed']),['tools/pr_policy.py'],True,ok,(True,None)),
        ('security',event(['security-reviewed'],sha='bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'),['tools/pr_policy.py'],True,ok,(False,'security-reviewed')),
        ('security',event(['security-reviewed'],base='dev'),['tools/pr_policy.py'],True,ok,(False,'security-reviewed')),
        ('security',event(['security-reviewed']),['tools/pr_policy.py'],True,{'accepted':{('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','main')}},(False,'security-reviewed'))]:
    passed,_,drop=decide(check,case,paths,complete,found)
    if (passed,drop)!=want:errors.append('PR gate changed: %s %s %s'%(check,case['pull_request']['head']['sha'][:1],paths))
checksum=ROOT/'checksums.json'
if a.write_checksums:
    if errors:raise SystemExit(json.dumps({'passed':False,'errors':errors}))
    checksum.write_text(json.dumps(inventory,indent=2,sort_keys=True)+'\n')
elif not checksum.exists() or json.loads(checksum.read_text())!=inventory:
    errors.append('source checksum inventory differs; review intended edits, then regenerate')
result={'passed':not errors,'regular_source_files':len(inventory),'local_links_checked':links,
    'shared_skill_link':alias.is_symlink() and alias.resolve()==(ROOT/'.agents/skills').resolve(),
    'checksums_updated':a.write_checksums,'errors':errors,'scope':'Source/layout integrity only; no model, runtime, platform-compatibility or production result.'}
print(json.dumps(result,indent=2));raise SystemExit(bool(errors))
