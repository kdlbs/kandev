import {test} from 'node:test';
import assert from 'node:assert/strict';
import { assertComplete, parseOptions } from './run.mjs';
const titles = ['baseline seed provenance', 'preserves release data and replays startup', 'executes old and new task lifecycles after upgrade'];
const report = () => ({suites:[{specs:titles.map(title => ({title, tests:[{projectName:'upgrade-desktop', results:[{status:'passed'}]}]}))}], errors:[]});
test('requires every named scenario to execute and pass with no skipped/failing attempt', () => {
  assert.doesNotThrow(() => assertComplete(report(),'upgrade-desktop'));
  for (const status of ['skipped','failed','timedOut','interrupted']) {
    const value=report(); value.suites[0].specs[0].tests[0].results[0].status=status;
    assert.throws(() => assertComplete(value,'upgrade-desktop'), /scenario/);
  }
  const missing=report();missing.suites[0].specs.pop();
  assert.throws(() => assertComplete(missing,'upgrade-desktop'), /scenario/);
  const wrong=report();wrong.suites[0].specs[0].tests[0].projectName='chromium';
  assert.throws(() => assertComplete(wrong,'upgrade-desktop'), /scenario/);
  const error=report();error.errors=[{message:'worker crashed'}];
  assert.throws(() => assertComplete(error,'upgrade-desktop'), /worker/);
});
test('rejects invalid engine, project, tag and incomplete arguments', () => {
  assert.deepEqual(parseOptions([]),{baseline:'latest',engine:'sqlite',project:'upgrade-desktop',noBuild:false});
  for (const args of [['--engine','mysql'],['--project','chromium'],['--baseline','main'],['--engine'],['--wat'],['--engine','postgres','--project','upgrade-mobile']]) assert.throws(() => parseOptions(args));
});

test('resource cleanup runs even when the child command fails', async () => {
  const {withCleanup}=await import('./run.mjs');
  const events=[];
  await assert.rejects(withCleanup(async () => {events.push('run');throw new Error('child exit 1');},async () => {events.push('cleanup');}), /child exit 1/);
  assert.deepEqual(events,['run','cleanup']);
});
