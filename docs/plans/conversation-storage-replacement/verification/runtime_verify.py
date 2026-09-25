import os, subprocess, time, json, urllib.request, concurrent.futures, pathlib, socket, sqlite3, sys
root=pathlib.Path(sys.argv[1]).resolve()
if not (root/'data/data/kandev.db').is_file(): raise SystemExit('Expected disposable large-upgrade fixture at <root>/data/data/kandev.db')
home=root/'runtime-home';home.mkdir(exist_ok=True)
legacy_host=home/'plugins/.host';legacy_host.mkdir(parents=True,exist_ok=True)
with sqlite3.connect(legacy_host/'session-events.sqlite') as connection:
 connection.execute('CREATE TABLE IF NOT EXISTS legacy_fixture(payload TEXT)')
 connection.execute("INSERT INTO legacy_fixture VALUES('synthetic legacy host journal')")
for suffix in ['-wal','-shm']:(legacy_host/('session-events.sqlite'+suffix)).touch()
# OS-selected loopback port; only this disposable backend owns the fixture.
sock=socket.socket();sock.bind(('127.0.0.1',0));port=sock.getsockname()[1];sock.close()
env={'PATH':str(root)+os.pathsep+os.environ['PATH'],'HOME':str(home),'KANDEV_HOME_DIR':str(home),'KANDEV_DATABASE_PATH':str(root/'data/data/kandev.db'),'KANDEV_SERVER_HOST':'127.0.0.1','KANDEV_SERVER_PORT':str(port),'KANDEV_AGENT_STANDALONE_PORT':str(port+1),'KANDEV_E2E_MOCK':'true','KANDEV_MOCK_AGENT':'true','KANDEV_MOCK_GITHUB':'true','KANDEV_MOCK_JIRA':'true','KANDEV_MOCK_LINEAR':'true','KANDEV_DEBUG_PPROF_ENABLED':'true','KANDEV_LOG_LEVEL':'info'}
log=open(root/'runtime.log','w');started=time.monotonic();p=subprocess.Popen([str(root/'kandev'),'__backend'],env=env,cwd=str(home),stdout=log,stderr=subprocess.STDOUT)
(root/'runtime.pid').write_text(str(p.pid))
base=f'http://127.0.0.1:{port}'
def request(path,data=None):
 req=urllib.request.Request(base+path,data=None if data is None else json.dumps(data).encode(),headers={'Content-Type':'application/json'},method='GET' if data is None else 'PATCH')
 with urllib.request.urlopen(req,timeout=5) as r: return r.status,r.read()
try:
 while time.monotonic()-started<180:
  if p.poll() is not None: raise RuntimeError(f'backend exited {p.returncode}')
  try:
   status,body=request('/ready')
   if status==200: break
  except Exception: pass
  time.sleep(.1)
 else: raise RuntimeError('readiness deadline')
 result={'process_to_http_ready_ms':round((time.monotonic()-started)*1000,3),'ready':json.loads(body),'workers':8,'duration_seconds':(0 if os.environ.get('KANDEV_VERIFY_READY_ONLY') else 60)}
 print(json.dumps(result),flush=True)
 deadline=time.monotonic()+(0 if os.environ.get('KANDEV_VERIFY_READY_ONLY') else 60)
 def stream(worker):
  success=0;errors=[];max_latency=0
  while time.monotonic()<deadline:
   start=time.monotonic()
   try: request(f'/api/v1/_test/messages/large-{worker}',{'content':'stream-'*1170+str(success)});success+=1
   except Exception as e: errors.append(str(e))
   latency=time.monotonic()-start;max_latency=max(max_latency,latency)
   time.sleep(max(0,.01-latency))
  return {'writes':success,'failures':len(errors),'errors':errors[:3],'max_write_ms':max_latency*1000}
 with concurrent.futures.ThreadPoolExecutor(max_workers=8) as executor:
  workers=[executor.submit(stream,i) for i in range(8)];checks=[]
  while time.monotonic()<deadline:
   start=time.monotonic()
   try:status,body=request('/ready');checks.append({'status':status,'ms':(time.monotonic()-start)*1000})
   except Exception as e: checks.append({'error':str(e)})
   time.sleep(.1)
  result['streams']=[w.result() for w in workers]
 result['remaining_legacy_host_files']=[str(p.name) for p in legacy_host.glob('session-events.sqlite*')];result['ready_samples']=len(checks);result['ready_failures']=[c for c in checks if c.get('status')!=200]
 result['max_ready_ms']=max((c.get('ms',0) for c in checks), default=0)
 try:
  _,body=request('/debug/pprof/goroutine?debug=2');(root/'goroutines.txt').write_bytes(body)
 except Exception as e:result['goroutine_capture_error']=str(e)
 (root/'runtime-results.json').write_text(json.dumps(result,indent=2));print(json.dumps(result),flush=True)
finally:
 p.terminate()
 try:p.wait(timeout=20)
 except subprocess.TimeoutExpired:p.kill();p.wait()
 log.close()
