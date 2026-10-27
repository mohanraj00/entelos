"""Invoke the pinned CEL executable with explicit bundles."""
import json
from pathlib import Path
import subprocess
import tempfile
from git_store import Fault, encode, decode

FILENAMES={'manifest':'manifest.json','environment':'env.json','source':'rule.cel','fixed':'fixed.json'}
MIMES={'manifest':'application/vnd.cel-handoff.bundle+json','environment':'application/json','source':'text/plain','fixed':'application/json'}

class CELRuntime:
    def __init__(self,binary,work):
        self.binary=str(Path(binary).resolve());self.work=Path(work).resolve()
        self.work.mkdir(parents=True,exist_ok=True)
    def call(self,args,data=None):
        try:
            p=subprocess.run([self.binary,*args],input=data,capture_output=True,timeout=15)
            result=decode(p.stdout)
        except (OSError,subprocess.TimeoutExpired,Fault) as e:
            raise Fault('implementation_unavailable','pinned CEL process unavailable: '+str(e),'runtime') from e
        return p.returncode,result
    def build(self):
        code,value=self.call(['-build'])
        if code or not value.get('implementation_ref'):raise Fault('implementation_unavailable','missing build identity','runtime')
        return value['implementation_ref']
    def execute(self,files,command,inputs=None,cases=None):
        with tempfile.TemporaryDirectory(dir=self.work) as directory:
            path=Path(directory)
            for key,filename in FILENAMES.items(): (path/filename).write_bytes(files[key])
            args=['-bundle',str(path)]
            if cases is not None:
                (path/'cases.json').write_bytes(encode(cases));args+=['-cases',str(path/'cases.json')]
            args+=[command]
            data=None
            if inputs is not None:
                manifest=decode(files['manifest'])
                from git_store import content_ref,envelope
                ref=content_ref(envelope(MIMES['manifest'],files['manifest']))
                data=encode({'bundle_ref':ref,'inputs':decode(inputs)})
            code,value=self.call(args,data)
            if command=='describe' and code:
                raise Fault(value.get('status','runtime_error'),value.get('diagnostic',{}).get('message','load failed'),'runtime')
            return value
