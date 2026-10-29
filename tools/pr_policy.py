#!/usr/bin/env python3
"""Enforce the PR acceptance and security-review gates. The PR policy workflow runs this file from the base branch, never from the PR."""
import argparse
import json
import os
from pathlib import Path
import sys
import urllib.error
import urllib.parse
import urllib.request

sys.path.insert(0,str(Path(__file__).resolve().parent))
from classify_change import classify

API='https://api.github.com'
# GitHub lists at most 3000 files for a PR. A longer list is treated as security-relevant.
PAGES=30

def call(method,path,token):
    request=urllib.request.Request(API+path,method=method,headers={'Authorization':'Bearer '+token,
        'Accept':'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28'})
    with urllib.request.urlopen(request,timeout=30) as response:return json.loads(response.read() or b'null')

def changed_paths(repo,number,token):
    # Both ends of a rename count, so a move out of a checked path still counts.
    paths=[]
    for page in range(1,PAGES+1):
        batch=call('GET','/repos/%s/pulls/%d/files?per_page=100&page=%d'%(repo,number,page),token)
        for item in batch:paths+=[item['filename']]+([item['previous_filename']] if item.get('previous_filename') else [])
        if len(batch)<100:return paths,True
    return paths,False

def remove_label(repo,number,label,token):
    try:call('DELETE','/repos/%s/issues/%d/labels/%s'%(repo,number,urllib.parse.quote(label)),token)
    except urllib.error.HTTPError as error:
        if error.code!=404:raise

def decide(check,event,paths,complete):
    """Return (passed, message, label to remove or None). This function makes no network calls."""
    # A push, a reopen or a new base can change the reviewed bytes, so each one removes an earlier approval.
    pr=event['pull_request'];labels={x['name'] for x in pr['labels']}
    push=event['action'] in ('synchronize','reopened') or (event['action']=='edited' and 'base' in event.get('changes',{}))
    head=(pr['head'].get('repo') or {}).get('full_name');base=event['repository']['full_name']
    if check=='acceptance':
        if head==base:return True,'Same-repository PR: no fork acceptance needed.',None
        drop='accepted' if push and 'accepted' in labels else None
        if drop:return False,'The fork %s changed its commits or base. Do the acceptance review in docs/REVIEW.md again, then add the accepted label.'%head,drop
        if 'accepted' in labels:return True,'A maintainer accepted this fork PR.',None
        return False,'This PR comes from the fork %s. Do the acceptance review in docs/REVIEW.md, then add the accepted label.'%head,None
    change=classify(paths)
    if complete and not change['security']:return True,'No security-relevant paths changed.',None
    found='Security-relevant paths: '+(', '.join(change['security_paths']) if complete else 'the PR has more files than GitHub lists')
    drop='security-reviewed' if push and 'security-reviewed' in labels else None
    if drop:return False,found+'\nA push, reopen or base change removed the security-reviewed label. Review the current change, then add the label again.',drop
    if 'security-reviewed' in labels:return True,found+'\nThe security-reviewed label is present.',None
    return False,found+'\nRun tools/cross_review.py --security yes, post the result on the PR, then add the security-reviewed label.',None

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('check',choices=['acceptance','security'])
    a=p.parse_args();token=os.environ['GITHUB_TOKEN']
    event=json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text());repo=event['repository']['full_name'];number=event['pull_request']['number']
    paths,complete=changed_paths(repo,number,token) if a.check=='security' else ([],True)
    passed,message,drop=decide(a.check,event,paths,complete)
    if drop:remove_label(repo,number,drop,token)
    print(message)
    if not passed:print('::error::'+message.splitlines()[-1]);sys.exit(1)
