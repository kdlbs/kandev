import argparse
from pathlib import Path
parser=argparse.ArgumentParser()
parser.add_argument("--database",required=True)
parser.add_argument("--base-url",required=True)
args=parser.parse_args()
assert str(Path(args.database).resolve()).startswith("/tmp/kandev-iso-")
assert args.base_url.startswith(("http://localhost:","http://127.0.0.1:")) and not args.base_url.endswith(":38429")
import sqlite3,urllib.request,json,time,concurrent.futures
path=args.database
c=sqlite3.connect(path,timeout=30)
c.executescript('''CREATE TABLE IF NOT EXISTS journey_summary_audit(kind TEXT, task_id TEXT);
CREATE TRIGGER IF NOT EXISTS journey_summary_insert AFTER INSERT ON task_status_summaries BEGIN INSERT INTO journey_summary_audit VALUES('insert',NEW.task_id); END;
CREATE TRIGGER IF NOT EXISTS journey_summary_update AFTER UPDATE ON task_status_summaries BEGIN INSERT INTO journey_summary_audit VALUES('update',NEW.task_id); END;''')
base=args.base_url.rstrip('/');route='/?workflowId=journey-workflow-0'
def request(route):
 t=time.perf_counter()
 with urllib.request.urlopen(base+route,timeout=30) as r: data=r.read();status=r.status
 return {'route':route,'ms':(time.perf_counter()-t)*1000,'bytes':len(data),'status':status}
results={}
# No browser remains on the instance during this probe.
c.execute('DELETE FROM journey_summary_audit');c.commit()
results['warm']=request(route);results['warm']['summary_mutations']=c.execute('SELECT kind,count(*) FROM journey_summary_audit GROUP BY kind').fetchall()
c.execute("DELETE FROM task_status_summaries WHERE task_id LIKE 'journey-%'");c.execute('DELETE FROM journey_summary_audit');c.commit()
results['cold_missing_summaries']=request(route);results['cold_missing_summaries']['summary_mutations']=c.execute('SELECT kind,count(*) FROM journey_summary_audit GROUP BY kind').fetchall()
c.execute('DELETE FROM journey_summary_audit');c.commit()
results['warm_after_repair']=request(route);results['warm_after_repair']['summary_mutations']=c.execute('SELECT kind,count(*) FROM journey_summary_audit GROUP BY kind').fetchall()
results['session_metadata_bytes']=c.execute('SELECT SUM(length(metadata)) FROM task_sessions').fetchone()[0]
# Hold only the synthetic database writer. Warm reads should complete before release.
c.execute('BEGIN IMMEDIATE')
with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
 f=pool.submit(request,route)
 time.sleep(1)
 results['warm_under_external_writer']={'completed_before_release':f.done(),'hold_ms':1000}
 c.rollback();results['warm_under_external_writer'].update(f.result())
c.executescript('DROP TRIGGER journey_summary_insert; DROP TRIGGER journey_summary_update; DROP TABLE journey_summary_audit;')
c.close();print(json.dumps(results,indent=2))
