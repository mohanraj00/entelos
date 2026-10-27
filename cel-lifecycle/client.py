#!/usr/bin/env python3
"""A caller needs only discovery, ordinary inputs, and a retry key, not CEL/Git data."""
import argparse
from pathlib import Path
import subprocess
import sys
from git_store import encode,decode

p=argparse.ArgumentParser(description=__doc__)
p.add_argument('--repo',required=True);p.add_argument('--context',required=True)
p.add_argument('--runtime',required=True);p.add_argument('--work',required=True)
p.add_argument('--key',required=True);p.add_argument('--function',default='record.visible')
p.add_argument('--date',default='2026-11-15')
p.add_argument('--inputs',default='{"state":"reviewed","archived":false}')
a=p.parse_args()
command=[sys.executable,'-B',str(Path(__file__).with_name('cli.py')),'--repo',a.repo,'--context',a.context,'--runtime',a.runtime,'--work',a.work]
def call(request):return decode(subprocess.check_output(command,input=encode(request)))
described=call({'op':'describe','function':a.function,'date':a.date})
if described['status']!='ok':print(encode(described).decode(),end='');raise SystemExit(1)
descriptor=described['data']
request={'op':'invoke','function':a.function,'date':a.date,'inputs':decode(a.inputs.encode()),
         'expected_bundle_ref':descriptor['callable']['bundle_ref'],'expected_revision':descriptor['cursor'],'request_key':a.key}
response=call(request)
sys.stdout.buffer.write(encode(response));raise SystemExit(response['status']!='ok')
