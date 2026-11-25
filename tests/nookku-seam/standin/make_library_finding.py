"""Write the stand-in finding record of the library example. This file is temporary.

The seam test of #112 needs a finding record of a released nookku version. The release with
the finding record is not published yet. Thus a person ran this file one time with the nookku
source of one commit, and the output is library_finding.jsonl. SOURCE.json records the commit
and the SHA-256 of the output. CI does not run this file, and CI installs no nookku.

After the release, the seam test runs the same steps with the nookku wheel that
tests/nookku-seam/requirements.txt pins by hash, and this folder goes.

The example is invented. An agent answers questions about the due dates of a sample library
catalog. The catalog gives a wrong due date for one title. The answer key has the right date,
a person adjudicates the mismatch as agent_wrong, and nookku exports one finding record.

usage: PYTHONPATH=<nookku source>/src python3 -B make_library_finding.py <work folder> <output file>
"""
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import sys

from nookku import bridge
from nookku.cli import main

MOSS = 'When is Field Notes on Moss due?'
GARDEN = 'When is The Night Garden due?'
# The catalog that the agent reads. The due date of The Night Garden is wrong.
OLD_CATALOG = '{"due": {"Field Notes on Moss": "2026-12-04", "The Night Garden": "2026-12-16"}}'
# The loan list of the library, as an answer key. A person wrote these dates.
KEY = {'source': 'the loan list of the sample library', 'version': '1',
       'answers': [{'question': MOSS, 'answer': '2026-12-04'}, {'question': GARDEN, 'answer': '2026-12-09'}]}
SPEC = '''# Sample library

## Answer key

| File | Field | Match |
|---|---|---|
| `keys/due.json` | `due` | exact |
'''
EGRESS = {'model_apis': [], 'backends': [], 'mcp_servers': [], 'databases': [], 'other': []}
# The agent speaks the agent contract of nookku: one JSON line in, one JSON line out.
AGENT = '''
import json, sys
due = json.load(open(sys.argv[1]))["due"]
for raw in sys.stdin.buffer:
    request = json.loads(raw)
    title = next((t for t in sorted(due) if t in request["message"]), None)
    reply = {"reply": "I do not know that title."} if title is None else {
        "due": due[title], "reply": title + " is due on " + due[title] + "."}
    sys.stdout.write(json.dumps({"v": 1, "id": request["id"], "reply": json.dumps(reply)}) + "\\n")
    sys.stdout.flush()
'''


def cli(*words):
    # The export writes bytes, so the standard output of a command has a buffer.
    raw = io.BytesIO()
    out = io.TextIOWrapper(raw, encoding='utf-8', newline='')
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(io.StringIO()):
        code = main([str(w) for w in words])
    out.flush()
    return code, raw.getvalue().decode('utf-8')


def export(work):
    """Run the library test in a new project folder, and give the exported finding lines."""
    # Each state file of nookku and of a harness stays in the work folder.
    for name in ['NOOKKU_HOME', 'CODEX_HOME', 'CLAUDE_CONFIG_DIR']:
        (work / name.lower()).mkdir()
        os.environ[name] = str(work / name.lower())
    os.environ['NOOKKU_CODEX'] = 'no-codex-command'
    root = work / 'library'
    (root / '.nookku').mkdir(parents=True)
    (root / 'keys').mkdir()
    (work / 'agent.py').write_text(AGENT, 'utf-8')
    (work / 'catalog.json').write_text(OLD_CATALOG, 'utf-8')
    (root / 'APP_SPEC.md').write_text(SPEC, 'utf-8')
    (root / 'keys' / 'due.json').write_text(json.dumps(KEY), 'utf-8')
    config = {'entry': [sys.executable, str(work / 'agent.py'), str(work / 'catalog.json')], 'models': [],
              'spec': ['APP_SPEC.md'], 'page': False}
    (root / '.nookku' / 'config.json').write_text(json.dumps(config), 'utf-8')
    (root / 'script.jsonl').write_text(''.join(json.dumps({'said': s}) + '\n' for s in [MOSS, GARDEN]), 'utf-8')
    # The agent calls no service, so each list of the egress answer is empty.
    (root / '.nookku' / 'egress.json').write_text(json.dumps(EGRESS), 'utf-8')
    code, out = cli('register', '--root', root, '--file', root / '.nookku' / 'egress.json')
    assert code == 0, out
    code, out = cli('run', '--script', root / 'script.jsonl', '--root', root, '--json')
    folder = bridge.latest_test(root)
    assert folder is not None, out
    code, out = cli('adjudicate', folder.name, '--root', root, '--author', 'Asha', '--turn', '2',
                    '--outcome', 'agent_wrong', '--reason', 'The loan list gives 2026-12-09.')
    assert code == 0, out
    subject = 'sha256:' + hashlib.sha256(OLD_CATALOG.encode('utf-8')).hexdigest()
    code, out = cli('finding', 'export', folder.name, '--root', root, '--subject', subject)
    assert code == 0, out
    return out


if __name__ == '__main__':
    lines = export(Path(sys.argv[1]))
    assert len(lines.splitlines()) == 1, lines
    Path(sys.argv[2]).write_text(lines, 'utf-8')
