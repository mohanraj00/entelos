#!/usr/bin/env python3
"""Discover and call a host-selected function. No CEL source or fixed data is read."""
import json
import subprocess
import sys

binary, bundle = sys.argv[1:3]
descriptor = json.loads(subprocess.check_output([binary, '-bundle', bundle, 'describe']))
request = {'bundle_ref': descriptor['bundle_ref'], 'inputs': {'state': 'ready', 'archived': False}}
result = subprocess.check_output([binary, '-bundle', bundle, 'run'], input=(json.dumps(request) + '\n').encode())
sys.stdout.buffer.write(result)
