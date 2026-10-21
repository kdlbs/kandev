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
  assert.deepEqual(workflow.on.push.branches,['main']);
  assert.equal(workflow.on.schedule.length,1);
  for (const job of Object.values(workflow.jobs)) {
    assert.equal(job.if, "github.ref == 'refs/heads/main'");
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
  const evidence = testJob.steps.find(s => s.if === 'always()' && s.uses?.startsWith('actions/upload-artifact@'));
  assert.equal(evidence.with['include-hidden-files'], true);
  assert.ok(evidence.with.path.split('\n').includes('apps/web/e2e/test-results'));
  for (const job of Object.values(workflow.jobs)) {
    assert.equal(job.steps.find(s => s.uses?.startsWith('actions/setup-go@')).with['cache-dependency-path'], 'apps/backend/go.sum');
  }
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


test('upgrade jobs always publish migration identity and test summaries, including failure paths', () => {
  const workflow=parse(fs.readFileSync(workflowPath,'utf8'));
  for (const job of Object.values(workflow.jobs)) {
    const summary=job.steps.find(s => s.name === 'Publish upgrade summary');
    assert.equal(summary.if,'always()');
    assert.equal(summary.run,'node scripts/upgrade/summary.mjs');
    assert.equal(summary.env.UPGRADE_JOB_OUTCOME,'${{ job.status }}');
  }
  assert.ok(workflow.jobs.rehearse.steps.find(s => s.name === 'Check harness contracts').run.includes('scripts/upgrade/summary.test.mjs'));
});
