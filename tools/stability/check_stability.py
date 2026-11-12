#!/usr/bin/env python3
"""Pin the stable surfaces of docs/STABILITY.md: the Go API, the wire files and the version values.

The check fails on each difference from the pins in this directory. A planned
change updates the pins with --write in the same PR. See docs/STABILITY.md.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[2]
HERE=ROOT/'tools/stability'
CLASSES=('stable','experimental','deprecated')

def go_api(pins):
    go=shutil.which('go')
    if not go:return None,['go is not on the PATH; the Go API pin needs Go to read the source']
    args=[go,'run',str(HERE/'apidump.go'),str(ROOT)]+['%s=%s'%(k,v) for k,v in sorted(pins['packages'].items())]
    run=subprocess.run(args,capture_output=True,text=True,cwd=HERE)
    if run.returncode:return None,['apidump failed: '+run.stderr.strip()]
    return run.stdout.splitlines(),[]

def stable_files(pins):
    return {p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest()
        for pattern in pins['files'] for p in sorted(ROOT.glob(pattern)) if p.is_file()}

def read_api():
    pinned={}
    for n,raw in enumerate((HERE/'api.txt').read_text().splitlines(),1):
        if not raw or raw.startswith('#'):continue
        cls,_,line=raw.partition(' ')
        if cls not in CLASSES:raise SystemExit('api.txt line %d: class must be one of %s'%(n,', '.join(CLASSES)))
        pinned[line]=cls
    return pinned

def name_of(line):
    """Give the CHANGELOG name of a declaration, for example lifecycle-core.Host.As."""
    path,kind,text=line.split(' ',2)
    recv=''
    if kind=='func' and text.startswith('('):
        inner,_,text=text[1:].partition(') ')
        recv=inner.lstrip('*').split('[')[0]+'.'
    name=text.replace('(',' ').replace('[',' ').split(' ',1)[0]
    return path.rsplit('/',1)[-1]+'.'+recv+name

def errors():
    pins=json.loads((HERE/'pins.json').read_text())
    lines,errs=go_api(pins)
    if lines is not None:
        pinned=read_api()
        for line in sorted(set(pinned)-set(lines)):errs.append('Go API removed or changed: %s %s'%(pinned[line],line))
        for line in sorted(set(lines)-set(pinned)):errs.append('Go API added or changed: '+line)
        log=(ROOT/'CHANGELOG.md').read_text()
        for line,cls in sorted(pinned.items()):
            if cls=='deprecated' and '- Deprecated: `%s`'%name_of(line) not in log:
                errs.append('deprecated %s has no "- Deprecated: `%s`" line in CHANGELOG.md'%(line,name_of(line)))
        for name,value in pins['constants'].items():
            path,_,const=name.rpartition('.')
            if 'const %s = "%s"'%(const,value) not in {l.split(' ',1)[1] for l in lines if l.startswith(path+' ')}:
                errs.append('constant %s is not "%s"'%(name,value))
    got=stable_files(pins)
    for name in sorted(set(pins['sha256'])|set(got)):
        if pins['sha256'].get(name)!=got.get(name):errs.append('stable file added, removed or changed: '+name)
    envelope=json.loads((ROOT/'src/protocol/schema/envelope.schema.json').read_text())
    versions=sorted({v for d in envelope.get('$defs',{}).values() if isinstance(d,dict)
        for v in d.get('properties',{}).get('protocol',{}).get('enum',[])})
    if versions!=pins['protocol_versions']:errs.append('envelope protocol versions %s are not %s'%(versions,pins['protocol_versions']))
    return errs

def write():
    pins=json.loads((HERE/'pins.json').read_text())
    lines,errs=go_api(pins)
    if errs:raise SystemExit('\n'.join(errs))
    old=read_api()
    gone=[l for l,c in old.items() if c=='stable' and l not in lines]
    if gone:raise SystemExit('a stable declaration is gone; mark it deprecated first (docs/STABILITY.md):\n'+'\n'.join(gone))
    # A new declaration starts as experimental. A promotion to stable is a separate edit.
    body=['%s %s'%(old.get(l,'experimental'),l) for l in lines]
    head=[l for l in (HERE/'api.txt').read_text().splitlines() if l.startswith('#')]
    (HERE/'api.txt').write_text('\n'.join(head+body)+'\n')
    pins['sha256']=stable_files(pins)
    (HERE/'pins.json').write_text(json.dumps(pins,indent=2)+'\n')

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--write',action='store_true',help='record the current surfaces as the pins')
    if p.parse_args().write:write()
    errs=errors()
    print(json.dumps({'passed':not errs,'errors':errs},indent=2));sys.exit(bool(errs))
