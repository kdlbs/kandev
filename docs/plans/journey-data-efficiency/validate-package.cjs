const fs=require('fs');const path=require('path');
const root=process.cwd();const {validateCoverage}=require(path.join(root,'.github/scripts/pr-docs.cjs'));
const dir='docs/plans/journey-data-efficiency';
const orderPaths=fs.readdirSync(dir).filter(n=>/^task-.*\.md$/.test(n)).map(n=>dir+'/'+n);
const docs=[...orderPaths,dir+'/plan.md','docs/specs/platform/system-design/journey-data-loading.md','docs/specs/platform/requirements/journey-data-loading.md','docs/specs/platform/requirements/interactive-read-availability.md','docs/specs/platform/requirements/bounded-task-status-delivery.md'];
const fileContents=Object.fromEntries(docs.map(p=>[p,fs.readFileSync(p,'utf8')]));
const changedFiles=docs.map(filename=>({filename,status:'modified'}));
// The representative production path exercises implementation work-order coverage.
changedFiles.push({filename:'apps/backend/internal/task/repository/sqlite/completion_gates.go',status:'modified'});
const result=validateCoverage({changedFiles,fileContents});
console.log(JSON.stringify({ok:result.ok,status:result.status,workOrders:result.workOrders,errors:result.errors},null,2));
if(!result.ok)process.exitCode=1;
