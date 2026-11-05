// Bounded synthetic work. The host verifies identities before cancelling it.
const fs = require('node:fs');
const cp = require('node:child_process');
const prefix = process.argv[2];
const child = cp.spawn(process.execPath, ['-e',
  "const fs=require('node:fs');setInterval(()=>fs.writeFileSync(process.argv[1]+'-detached',String(Date.now())),200);setTimeout(()=>process.exit(0),600000)", prefix,
], { detached: true, stdio: 'ignore' });
child.unref();
fs.writeFileSync(prefix + '-started.json', JSON.stringify({ parent: process.pid, detached: child.pid }));
setInterval(() => fs.writeFileSync(prefix + '-heartbeat', String(Date.now())), 200);
setTimeout(() => process.exit(0), 600000);
