async page => {
 await page.context().addInitScript(() => {
  const probe = window.__journeyProbe = { frames: [], fetches: [] };
  const Native = window.WebSocket;
  window.WebSocket = class extends Native {
   constructor(...args) {
    super(...args);
    this.addEventListener('message', e => record('in', e.data));
   }
   send(data) { record('out', data); return super.send(data); }
  };
  function record(direction, data) {
   try { const m=JSON.parse(data); probe.frames.push({direction, action:m.action, session_id:m.payload?.session_id, task_id:m.payload?.task_id, bytes:typeof data==='string'?new TextEncoder().encode(data).length:0}); } catch {}
  }
  const nativeFetch=window.fetch;
  window.fetch=async(...args)=>{
   const start=performance.now();
   const response=await nativeFetch(...args);
   const path=typeof args[0]==='string'?args[0]:args[0]?.url;
   probe.fetches.push({path,status:response.status,ms:performance.now()-start});
   return response;
  };
 });
 console.log('journey recorder installed');
}
