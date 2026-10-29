#!/usr/bin/env python3
"""Enforce the PR acceptance and security-review gates. The PR policy workflow runs this file from the base branch, never from the PR."""
import argparse
import json
import os
import re
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

def call(method,path,token,body=None):
    request=urllib.request.Request(API+path,method=method,data=json.dumps(body).encode() if body is not None else None,headers={'Authorization':'Bearer '+token,
        'Accept':'application/vnd.github+json','X-GitHub-Api-Version':'2022-11-28','Content-Type':'application/json'})
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

def live_pr(repo,number,token):
    # The event payload can be older than a later push, so the gate reads the current labels, head and base.
    pr=call('GET','/repos/%s/pulls/%d'%(repo,number),token)
    return {'labels':[{'name':x['name']} for x in pr['labels']],'head':{'sha':pr['head']['sha'],'repo':pr['head']['repo']},'base':{'ref':pr['base']['ref']}}

# The workflow token posts as this user. A fork PR gets a read-only token, so it cannot post as this user.
# A writer can change a workflow to post as this user, so the gates trust writers.
BOT='github-actions[bot]'
BINDING=re.compile(r'<!-- pr-policy:approval label=(\S+) head=([0-9a-f]{40}) base=(\S+) -->')
# The Codex GitHub integration posts its review results as this app. A user cannot post as an app.
CODEX='chatgpt-codex-connector[bot]'
REVIEWED=re.compile(r'\*\*Reviewed commit:\*\* `([0-9a-f]{7,40})`')

def records(repo,number,token):
    """Return ({label: {(head, base ref)}}, [(commit prefix, clean)]) from the approval records of this workflow and the Codex security review results on the PR."""
    found={};reviews=[]
    for page in range(1,PAGES+1):
        batch=call('GET','/repos/%s/issues/%d/comments?per_page=100&page=%d'%(repo,number,page),token)
        for item in batch:
            body=item['body'] or ''
            if item['user']['login']==BOT:
                for label,head,base in BINDING.findall(body):found.setdefault(label,set()).add((head,base))
            # A result without the clean text counts as open findings.
            elif item['user']['login']==CODEX and 'Codex Security Review' in body:
                reviews+=[(commit,'No security issues were found' in body) for commit in REVIEWED.findall(body)]
        if len(batch)<100:break
    return found,reviews

def bind(repo,number,label,pr,token):
    # The labeled event holds the head and base at the time of the label, not at the time of this run.
    head,base=pr['head']['sha'],pr['base']['ref']
    body='<!-- pr-policy:approval label=%s head=%s base=%s -->\nThe `%s` label applies to head %s on base `%s` only. A new head or base needs the label again.'%(label,head,base,label,head[:12],base)
    call('POST','/repos/%s/issues/%d/comments'%(repo,number),token,{'body':body})
    return head,base

def decide(check,event,paths,complete,found,reviews=()):
    """Return (passed, message, label to remove or None). event holds the live PR. This function makes no network calls."""
    # A label is valid only with an approval record for the current head and base, so a lost or replaced event cannot keep an old approval.
    pr=event['pull_request'];labels={x['name'] for x in pr['labels']}
    current=(pr['head']['sha'],pr['base']['ref'])
    head=(pr['head'].get('repo') or {}).get('full_name');base=event['repository']['full_name']
    label='accepted' if check=='acceptance' else 'security-reviewed'
    valid=label in labels and current in found.get(label,set())
    stale=label if label in labels and not valid else None
    again='The %s label has no approval record for head %s on base %s. Review the current change, then add the label again.'%(label,current[0][:12],current[1])
    if check=='acceptance':
        if head==base:return True,'Same-repository PR: no fork acceptance needed.',None
        if stale:return False,'The fork %s changed after the acceptance review in docs/REVIEW.md.\n%s'%(head,again),stale
        if valid:return True,'A maintainer accepted this fork PR at head %s.'%current[0][:12],None
        return False,'This PR comes from the fork %s. Do the acceptance review in docs/REVIEW.md, then add the accepted label.'%head,None
    change=classify(paths)
    if complete and not change['security']:return True,'No security-relevant paths changed.',None
    found_paths='Security-relevant paths: '+(', '.join(change['security_paths']) if complete else 'the PR has more files than GitHub lists')
    # Comments come oldest first. The latest Codex security review of the current head passes without a label if it found no issues.
    results=[clean for commit,clean in reviews if current[0].startswith(commit)]
    if results and results[-1]:
        return True,found_paths+'\nThe Codex security review of head %s found no security issues.'%current[0][:12],stale
    if stale:return False,found_paths+'\n'+again,stale
    if valid:return True,found_paths+'\nThe security-reviewed label applies to head %s.'%current[0][:12],None
    return False,found_paths+'\nRequest a security review as docs/REVIEW.md says. The check passes when the Codex security review of head %s finds no security issues, or when the maintainer adds the security-reviewed label.'%current[0][:12],None

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('check',choices=['acceptance','security'])
    a=p.parse_args();token=os.environ['GITHUB_TOKEN']
    event=json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text());repo=event['repository']['full_name'];number=event['pull_request']['number']
    found,reviews=records(repo,number,token);label='accepted' if a.check=='acceptance' else 'security-reviewed'
    if event['action']=='labeled' and event['label']['name']==label:found.setdefault(label,set()).add(bind(repo,number,label,event['pull_request'],token))
    event['pull_request']=live_pr(repo,number,token)
    paths,complete=changed_paths(repo,number,token) if a.check=='security' else ([],True)
    passed,message,drop=decide(a.check,event,paths,complete,found,reviews)
    if drop:remove_label(repo,number,drop,token)
    print(message)
    if not passed:print('::error::'+message.splitlines()[-1]);sys.exit(1)
