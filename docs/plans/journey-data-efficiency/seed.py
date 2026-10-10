#!/usr/bin/env python3
"""Populate only a dev-isolated database with deterministic synthetic journey data."""
import argparse
import datetime
import json
from pathlib import Path
import sqlite3

parser = argparse.ArgumentParser()
parser.add_argument('--database', required=True)
parser.add_argument('--tasks', type=int, choices=[10, 1000, 10000], required=True)
a = parser.parse_args()
p = Path(a.database).resolve()
assert str(p).startswith('/tmp/kandev-iso-'), 'Only a dev-isolated database is accepted'
assert p.name == 'kandev.db' and p.is_file()
c = sqlite3.connect(p, timeout=30)
c.execute('PRAGMA foreign_keys=ON')
assert c.execute("select count(*) from tasks where id not like 'journey-%'").fetchone()[0] == 0, 'Database contains non-fixture tasks'
existing_tasks = c.execute("select count(*) from tasks where id like 'journey-task-%'").fetchone()[0]
assert existing_tasks <= a.tasks, 'Fixture cannot shrink'
if a.tasks == 10000:
 assert existing_tasks == 1000, 'The 10,000-task comparison must expand the 1,000-task fixture'
workspace = c.execute('select id from workspaces order by created_at limit 1').fetchone()[0]
profile = c.execute('select id from agent_profiles where enabled=1 limit 1').fetchone()[0]
now = datetime.datetime.now(datetime.timezone.utc)
stamp = now.isoformat().replace('+00:00', 'Z')
workflows = []
for j in range(3):
 w = 'journey-workflow-'+str(j)
 c.execute('insert or ignore into workflows(id,workspace_id,name,created_at,updated_at) values(?,?,?,?,?)',(w,workspace,'Journey board '+str(j),stamp,stamp))
 c.execute('insert or ignore into workflow_steps(id,workflow_id,name,is_start_step,created_at,updated_at) values(?,?,?,1,?,?)',(w+'-todo',w,'Ready',stamp,stamp))
 workflows.append(w)
for i in range(existing_tasks, a.tasks):
 task = 'journey-task-'+str(i).zfill(4)
 w = workflows[2] if a.tasks == 10000 else workflows[0 if i < a.tasks//2 else 1 if i < 3*a.tasks//4 else 2]
 c.execute('insert or ignore into tasks(id,workspace_id,workflow_id,workflow_step_id,title,description,created_at,updated_at) values(?,?,?,?,?,?,?,?)',(task,workspace,w,w+'-todo',f'Journey task {i:04}', 'Synthetic task description. '*20,stamp,stamp))
 # The two target journeys have one and eight sessions. Other rows have two.
 count = 1 if i == 0 else 8 if i == 1 else 2
 for j in range(count):
  sid = task+'-session-'+str(j)
  c.execute('insert or ignore into task_sessions(id,task_id,agent_profile_id,name,state,is_primary,metadata,started_at,updated_at) values(?,?,?,?,?,?,?,?,?)',(sid,task,profile,f'Agent {j+1}','WAITING_FOR_INPUT',int(j==0),json.dumps({'journey_fixture':'x'*4096}),stamp,stamp))
  if i > 1: continue
  for n in range(200):
   tid=sid+'-turn-'+str(n)
   ts=(now-datetime.timedelta(minutes=201-n)).isoformat().replace('+00:00','Z')
   c.execute('insert or ignore into task_session_turns(id,task_session_id,task_id,started_at,completed_at,created_at,updated_at) values(?,?,?,?,?,?,?)',(tid,sid,task,ts,ts,ts,ts))
   for author in ['user','agent']:
    mid=tid+'-'+author
    content=f'Synthetic {author} message {n}. '+('data '*200 if author=='agent' else 'Please investigate the fixture.')
    c.execute('insert or ignore into task_session_messages(id,task_session_id,task_id,turn_id,author_type,content,created_at,updated_at) values(?,?,?,?,?,?,?,?)',(mid,sid,task,tid,author,content,ts,ts))
c.commit()
manifest={'workspace_id':workspace,'workflow_ids':workflows,'tasks':a.tasks,'targets':[{'task_id':'journey-task-0000','session_count':1},{'task_id':'journey-task-0001','session_count':8}],'rows':{t:c.execute('select count(*) from '+t).fetchone()[0] for t in ['tasks','task_sessions','task_session_turns','task_session_messages']}}
print(json.dumps(manifest,indent=2))
c.close()
