"""Locate the unchanged shared reference. This is application wiring, not a copied runtime."""
import os
from pathlib import Path
import sys
LIFE=Path(os.environ.get('CEL_LIFECYCLE_SOURCE',Path(__file__).resolve().parent.parent/'cel-lifecycle')).resolve()
sys.path.insert(0,str(LIFE))
from git_store import Fault,GitStore,encode,decode,content_ref,envelope,media
from cel_runtime import CELRuntime,MIMES
from lifecycle import Host
from scenario import Driver
ROOT=Path(__file__).resolve().parent

def fixture():return decode((ROOT/'fixtures/workflow.json').read_bytes())
