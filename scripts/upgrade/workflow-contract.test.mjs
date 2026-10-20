import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {createRequire} from 'node:module';
const require=createRequire(new URL('../../apps/web/package.json', import.meta.url));
const {parse}=require('yaml');
const workflowPath=new URL('../../.github/workflows/upgrade-regression.yml',import.meta.url);
test('upgrade CI is main-only and pins one candidate and baseline across the matrix', () => {
  assert.ok(fs.existsSync(workflowPath),'main-only upgrade workflow exists');
  const workflow=parse(fs.readFileSync(workflowPath,'utf8'));
  assert.deepEqual(Object.keys(workflow.on).sort(),['push','schedule','workflow_dispatch']);
  assert.deepEqual(workflow.on.push.branches,['main','feature/test-release-upgrade-7ee']);
  assert.equal(workflow.on.schedule.length,1);
  for (const job of Object.values(workflow.jobs)) {
    assert.match(job.if,/github.ref == 'refs\/heads\/main'/);
  }
  assert.equal(workflow.permissions.contents,'read');
  const prepare=workflow.jobs.prepare;
  assert.equal(prepare.steps.find(s => s.uses?.startsWith('actions/checkout@')).with.ref,'${{ github.sha }}');
  assert.equal(prepare.outputs.candidate_sha,'${{ steps.identity.outputs.candidate_sha }}');
  const testJob=workflow.jobs.rehearse;
  assert.equal(testJob.needs,'prepare');
  assert.equal(testJob.steps.find(s => s.uses?.startsWith('actions/checkout@')).with.ref,'${{ needs.prepare.outputs.candidate_sha }}');
  assert.deepEqual(testJob.strategy.matrix.include.map(row => [row.engine,row.project]),[
    ['sqlite','upgrade-desktop'],['postgres','upgrade-desktop'],['sqlite','upgrade-mobile'],
  ]);
  assert.ok(testJob.steps.some(s => s.uses?.startsWith('actions/download-artifact@')));
  assert.ok(testJob.steps.some(s => s.if === 'always()' && s.uses?.startsWith('actions/upload-artifact@')));
});


test('the managed runner guards upgrade projects before it can build', () => {
  const runner=fs.readFileSync(new URL('../../apps/web/e2e/scripts/run-e2e.sh',import.meta.url),'utf8');
  const gate=runner.indexOf('if [[ "$PROJECT" == upgrade-desktop || "$PROJECT" == upgrade-mobile ]]');
  const launch=runner.indexOf('log "mode=$MODE');
  assert.ok(gate >= 0 && gate < launch, 'upgrade gate precedes workload dispatch');
  const guard=runner.slice(gate,launch);
  for (const field of ['CI','GITHUB_ACTIONS','GITHUB_REF']) assert.ok(guard.includes(field));
  assert.ok(guard.includes('die '));
});
