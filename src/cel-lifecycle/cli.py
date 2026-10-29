#!/usr/bin/env python3
"""Host-selected local context and runtime; requests cannot supply authority."""
import argparse
from pathlib import Path
import sys
from git_store import GitStore,Fault,decode,encode
from cel_runtime import CELRuntime
from lifecycle import Host


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--repo',required=True,type=Path)
    p.add_argument('--context',required=True,type=Path,help='trusted host context, not caller input')
    p.add_argument('--runtime',required=True,type=Path)
    p.add_argument('--work',required=True,type=Path)
    a=p.parse_args()
    try:
        context=decode(a.context.read_bytes())
        host=Host(GitStore(a.repo),CELRuntime(a.runtime,a.work/'tmp'),context)
        while True:
            raw=sys.stdin.buffer.readline(1048577)
            if not raw:break
            if len(raw)>1048576:
                sys.stdout.buffer.write(encode(Fault('invalid_request','request exceeds reference byte bound; stream closed').record()));sys.stdout.buffer.flush();return 1
            try:
                result=host.handle(decode(raw))
                sys.stdout.buffer.write(encode({'status':'ok','data':result}))
            except Fault as e:sys.stdout.buffer.write(encode(e.record()))
            sys.stdout.buffer.flush()
    except (Fault,OSError) as e:
        value=e.record() if isinstance(e,Fault) else {'status':'configuration_error','message':str(e)}
        sys.stdout.buffer.write(encode(value));return 1
    return 0
if __name__=='__main__':raise SystemExit(main())
