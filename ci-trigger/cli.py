#!/usr/bin/env python3
"""Baseline-only ordinary-runtime host wiring. The lifecycle Host is unchanged."""
import argparse
from pathlib import Path
import sys
from support import GitStore,Host,Fault,encode,decode
from ordinary import OrdinaryRuntime
p=argparse.ArgumentParser(description=__doc__)
for name in ['repo','context','runtime','work']:p.add_argument('--'+name,required=True,type=Path)
a=p.parse_args()
try:
    host=Host(GitStore(a.repo),OrdinaryRuntime(),decode(a.context.read_bytes()))
    while True:
        raw=sys.stdin.buffer.readline(1048577)
        if not raw:break
        if len(raw)>1048576:
            sys.stdout.buffer.write(encode(Fault('invalid_request','request exceeds bound; stream closed').record()));sys.stdout.buffer.flush();raise SystemExit(1)
        try:
            result={'status':'ok','data':host.handle(decode(raw))}
        except Fault as e:result=e.record()
        sys.stdout.buffer.write(encode(result));sys.stdout.buffer.flush()
except (Fault,OSError) as e:
    sys.stdout.buffer.write(encode(e.record() if isinstance(e,Fault) else {'status':'configuration_error','message':str(e)}));raise SystemExit(1)
