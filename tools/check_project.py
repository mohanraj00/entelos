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
    'docs/STATUS.md','docs/BACKLOG.md','docs/CONTRACTS.md','docs/STANDARD.md','Makefile','.agents/skills/decision-workflow/SKILL.md',
    'docs/REVIEW.md','.github/CODEOWNERS','.github/workflows/ci.yml','.github/workflows/gates.yml','.github/workflows/gates-rerun.yml','.github/gates.json','.github/dependabot.yml','.github/workflows/claude-review.yml','.github/ISSUE_TEMPLATE/bug.yml','.github/ISSUE_TEMPLATE/workflow.yml','.github/ISSUE_TEMPLATE/config.yml']
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
for path,code,security in [('README.md',False,False),('src/lifecycle-core/host.go',True,True),
        ('src/lifecycle-core/gitstore.go',True,True),('src/lifecycle-core/client/lifecycle_client.py',True,True),('src/cel-handoff/handoff_test.go',True,False),('src/cel-handoff/go.sum',True,True),
        ('tools/classify_change.py',True,True),('Makefile',True,True),('docs/STANDARD.md',False,True),('.github/workflows/ci.yml',True,True),('.codex/config.toml',False,True),('GEMINI.md',False,True),
        ('docs/AGENTS.md',False,True),('src/cel-handoff/tools/prepare.py',True,True),('examples/ci-trigger/run.py',True,True),
        ('src/protocol/test_protocol.py',True,False),('src/lifecycle-core/scenario_test.go',True,False),('tests/test_export_boundaries.py',True,False),('docs/STATUS.md',False,False),('docs/REVIEW.md',False,True),('src/cel-lifecycle/.claude/settings.json',True,True),
        ('tests/AGENTS.md',True,True),('tests/CLAUDE.md',True,True),('tests/.claude/settings.json',True,True),('tests/go.mod',True,True),('src/cel-handoff/x_test.go',True,False)]:
    got=classify([path])
    if (got['code'],got['security'])!=(code,security):errors.append('classification changed for %s: code=%s security=%s'%(path,got['code'],got['security']))
# Review events default to the PR merge ref, so the Claude review must check out the default branch.
# The Claude review workflow keeps the design in docs/REVIEW.md. tools/claude_review.py owns the list of checks.
from claude_review import workflow_errors
errors+=['claude-review.yml '+x for x in workflow_errors((ROOT/'.github/workflows/claude-review.yml').read_text())]
# The licence record of each downloaded CI tool must name the pinned version.
cli=re.search(r'CLI_VERSION: (\S+)\n',(ROOT/'.github/workflows/claude-review.yml').read_text())
if not cli or '| @anthropic-ai/claude-code-linux-x64 | %s |'%cli.group(1) not in (ROOT/'licenses/THIRD_PARTY.md').read_text():errors.append('licenses/THIRD_PARTY.md must record the Claude CLI version of claude-review.yml')
lint=re.search(r'ACTIONLINT_VERSION: (\S+)\n',(ROOT/'.github/workflows/ci.yml').read_text())
if not lint or '| actionlint (rhysd/actionlint, linux_amd64 release archive) | %s |'%lint.group(1) not in (ROOT/'licenses/THIRD_PARTY.md').read_text():errors.append('licenses/THIRD_PARTY.md must record the actionlint version of ci.yml')
# The agy fallback must not load the maintainer's global allow rules or MCP servers.
review=(ROOT/'tools/cross_review.py').read_text()
for need in ["env['HOME']=home","'--sandbox'","'allow':[]"]:
    if need not in review:errors.append('cross_review.py must keep '+need)
if "'HOME'," in review:errors.append('cross_review.py must not pass the real HOME to agy')
# The shared gates read .github/gates.json from the base branch. They fail closed on an unknown key or a bad value.
try:
    gates=json.loads((ROOT/'.github/gates.json').read_text())
    keys={'code','security_sources','not_security','boundaries','level1','generated'}
    if gates.get('version')!=1 or set(gates)-keys-{'version'} or not all(isinstance(gates.get(k,[]),list) and all(isinstance(x,str) and x for x in gates.get(k,[])) for k in keys):
        errors.append('.github/gates.json must have "version": 1 and only lists of path patterns')
except (OSError,ValueError) as error:errors.append('.github/gates.json: %s'%error)
# Each gate action must use one full SHA of mohanraj00/.github, not a tag or a branch.
pins={m for f in sorted((ROOT/'.github/workflows').glob('*.yml')) for m in re.findall(r'uses: mohanraj00/\.github/gates@(\S+)',f.read_text())}
if len(pins)!=1 or not re.fullmatch(r'[0-9a-f]{40}',next(iter(pins),'')):errors.append('the gates action must use one full SHA in all workflows: %s'%sorted(pins))
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
