import argparse
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument("--database",required=True)
parser.add_argument("--base-url",required=True)
args=parser.parse_args()
assert str(Path(args.database).resolve()).startswith("/tmp/kandev-iso-")
assert args.base_url.startswith(("http://localhost:","http://127.0.0.1:")) and not args.base_url.endswith(":38429")
import sqlite3,urllib.request,time,concurrent.futures,json
c=sqlite3.connect(args.database);base=args.base_url.rstrip('/')
paths=['/?workflowId=journey-workflow-0','/api/v1/workflows/journey-workflow-0/snapshot','/api/v1/task-sessions/journey-task-0001-session-0']
def get(p):
 t=time.perf_counter()
 with urllib.request.urlopen(base+p,timeout=30) as r:r.read();status=r.status
 return {'route':p,'ms':(time.perf_counter()-t)*1000,'status':status}
c.execute('BEGIN IMMEDIATE')
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as ex:
 fs=[ex.submit(get,p) for p in paths];time.sleep(.5)
 stack=urllib.request.urlopen(base+'/debug/pprof/goroutine?debug=2').read()
 open('/tmp/kandev-journey-lock-stack.txt','wb').write(stack)
 done=[f.done() for f in fs];c.rollback()
 print(json.dumps([dict(f.result(),completed_before_release=d) for f,d in zip(fs,done)],indent=2))
