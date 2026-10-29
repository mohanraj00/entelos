"""Ordinary typed decision functions under the same lifecycle port, for comparison only."""
from pathlib import Path
import sys
from support import Fault,encode,decode,content_ref,envelope,MIMES

def initial(action:str,base_branch:str,draft:bool,target_branch:str)->bool:
    return not draft and base_branch==target_branch and action in ('opened','synchronize')
def amendment(action:str,base_branch:str,draft:bool,target_branch:str)->bool:
    return not draft and base_branch==target_branch and action in ('opened','synchronize','reopened')
def correction(action:str,base_branch:str,draft:bool,target_branch:str)->bool:
    return not draft and base_branch==target_branch and action in ('opened','synchronize','reopened','ready_for_review')
PROGRAMS={'ordinary:initial':initial,'ordinary:amendment':amendment,'ordinary:correction':correction}

def build_ref():
    return content_ref(envelope('application/vnd.ordinary-ci.implementation',Path(__file__).read_bytes()+sys.version.encode()))

class OrdinaryRuntime:
    def __init__(self,binary=None,work=None):pass
    def build(self):return build_ref()
    def execute(self,files,command,inputs=None,cases=None):
        m=decode(files['manifest']);env=decode(files['environment']);fixed=decode(files['fixed'])
        ref=content_ref(envelope(MIMES['manifest'],files['manifest']))
        if m['implementation_ref']!=self.build():raise Fault('implementation_unavailable','ordinary function build differs','ordinary-runtime')
        if m['profile']!='ordinary-ci/v1' or files['source'].decode().strip() not in PROGRAMS:raise Fault('unsupported_profile','only the frozen ordinary CI functions are installed','ordinary-runtime')
        for key,field in [('environment','environment_ref'),('source','source_ref'),('fixed','fixed_ref')]:
            if content_ref(envelope(MIMES[key],files[key]))!=m[field]:raise Fault('artifact_mismatch','ordinary closure differs','ordinary-runtime')
        types={v['name']:v['type'] for v in env['variables']};caller=m['caller_variables']
        if set(types)!={'action','base_branch','draft','target_branch'} or caller!=['action','base_branch','draft'] or set(fixed)!={'target_branch'} or not isinstance(fixed['target_branch'],str):raise Fault('invalid_bundle','ordinary CI contract differs','ordinary-runtime')
        descriptor={'function_id':m['function_id'],'bundle_ref':ref,'implementation_ref':self.build(),'profile':m['profile'],
                    'inputs':{'type':'object','properties':{k:{'type':'boolean' if types[k]=='bool' else 'string'} for k in caller},'required':caller,'additionalProperties':False},'output':{'type':'boolean'},'limits':m['limits']}
        if command=='describe':return descriptor
        def run(raw):
            result={'bundle_ref':ref,'implementation_ref':self.build()}
            def fail(code,field,message):return result|{'status':code,'diagnostic':{'code':code,'stage':'input','field':field,'message':message}}
            if len(raw)>m['limits']['max_request_bytes']:return fail('invalid_input','','request exceeds bound')
            obj=decode(raw)
            if not isinstance(obj,dict):return fail('invalid_input','','inputs must be an object')
            for name in sorted(obj):
                if name not in caller:return fail('invalid_input','/'+name,'unknown or fixed input')
            for name in caller:
                if name not in obj:return fail('needs_input','/'+name,'required field missing')
                value=obj[name]
                if (types[name]=='bool' and type(value) is not bool) or (types[name]=='string' and not isinstance(value,str)):return fail('invalid_input','/'+name,'wrong type; no coercion')
                if isinstance(value,str) and len(value.encode())>m['limits']['max_string_utf8_bytes']:return fail('invalid_input','/'+name,'string exceeds bound')
            value=PROGRAMS[files['source'].decode().strip()](**obj,**fixed)
            if type(value) is not bool:raise Fault('output_type','ordinary result must be bool')
            return result|{'status':'ok','value':value}
        if command=='run':return run(inputs)
        if command=='check':
            results=[];seen=set()
            for case in cases:
                actual=run(encode(case['inputs']));passed=bool(case.get('case')) and case['case'] not in seen and actual['status']==case['status']
                seen.add(case['case'])
                if case['status']=='ok':passed=passed and type(case.get('value')) is bool and actual.get('value') is case['value']
                else:passed=passed and 'value' not in actual and actual['diagnostic']['field']==case.get('field')
                results.append({'case':case['case'],'passed':passed,'actual':actual})
            return {'status':'passed' if results and all(x['passed'] for x in results) else 'failed','bundle_ref':ref,'implementation_ref':self.build(),'cases':results}
        raise Fault('invalid_operation','ordinary runtime operation is unsupported')
