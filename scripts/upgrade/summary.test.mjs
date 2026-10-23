import {test} from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import {renderSummary, writeSummary} from './summary.mjs';

const identity = {
  manifest:{tag:'v0.97.0',source_commit:'baseline-sha',archive_sha256:'verified-digest'},
  candidate:'candidate-sha', engine:'sqlite', project:'upgrade-desktop', outcome:'success',
};
const titles = ['baseline seed provenance', 'preserves release data and replays startup', 'executes old and new task lifecycles after upgrade'];
const report = statuses => ({suites:[{suites:[{specs:titles.map((title,i) => ({title,tests:[{
  projectName:'upgrade-desktop',results:(statuses[i] ?? []).map(status => ({status,duration:1250})),
}]}))}]}],errors:[]});

test('summary identifies both revisions and lists each nested scenario with its result and duration', () => {
  const text=renderSummary({...identity,report:report([['passed'],['passed'],['passed']])});
  for (const value of ['v0.97.0','baseline-sha','candidate-sha','verified-digest','sqlite','upgrade-desktop',...titles]) assert.ok(text.includes(value),value);
  assert.match(text,/3 passed/);
  assert.match(text,/1\.25 s/);
  assert.match(text,/upgrade-sqlite-upgrade-desktop/);
});

test('summary distinguishes failed, skipped, missing and retried tests without claiming acceptance', () => {
  const failed=renderSummary({...identity,outcome:'failure',report:report([['failed'],['skipped'],[]])});
  assert.match(failed,/1 failed/);
  assert.match(failed,/1 skipped/);
  assert.match(failed,/1 not run/);
  assert.match(failed,/Job result: failure/);
  const retried=renderSummary({...identity,report:report([['failed','passed'],['passed'],['passed']])});
  assert.match(retried,/retried/);
  assert.doesNotMatch(retried,/3 passed/);
  const missing=renderSummary({...identity,outcome:'failure'});
  assert.match(missing,/No Playwright report/);
  assert.match(missing,/3 not run/);
  for (const title of titles) assert.ok(missing.includes(title));
});

test('baseline summary includes planned matrix; unavailable identity and unsafe text stay explicit', () => {
  const text=renderSummary({candidate:'candidate-sha',outcome:'failure',notes:['bad <value>|`\nnext']});
  assert.match(text,/Baseline unavailable/);
  assert.match(text,/PostgreSQL/);
  assert.match(text,/upgrade-mobile/);
  assert.ok(text.includes('phone preserves release history'));
  assert.ok(text.includes('&lt;value&gt;'));
  assert.ok(!text.includes('bad <value>'));
});


test('summary entry point appends to the Actions summary even before a report or manifest exists', () => {
  const directory=fs.mkdtempSync(path.join(os.tmpdir(),'upgrade-summary-'));
  try {
    const output=path.join(directory,'summary.md');
    fs.writeFileSync(output,'Existing summary\n');
    writeSummary({GITHUB_STEP_SUMMARY:output,GITHUB_SHA:'candidate-sha',UPGRADE_PROJECT:'upgrade-mobile',UPGRADE_ENGINE:'sqlite',UPGRADE_JOB_OUTCOME:'failure'},directory);
    const text=fs.readFileSync(output,'utf8');
    assert.ok(text.startsWith('Existing summary\n'));
    assert.match(text,/Baseline unavailable/);
    assert.match(text,/1 not run/);
    const cache=path.join(directory,'.cache/upgrade/run-one');
    fs.mkdirSync(cache,{recursive:true});
    fs.writeFileSync(path.join(cache,'playwright.json'),JSON.stringify(report([['passed'],['failed'],['skipped']])));
    fs.writeFileSync(path.join(directory,'.cache/upgrade/manifest.json'),JSON.stringify(identity.manifest));
    writeSummary({GITHUB_STEP_SUMMARY:output,GITHUB_SHA:'candidate-sha',KANDEV_UPGRADE_MANIFEST:'.cache/upgrade/manifest.json',UPGRADE_PROJECT:'upgrade-desktop',UPGRADE_ENGINE:'sqlite',UPGRADE_JOB_OUTCOME:'failure'},directory);
    const appended=fs.readFileSync(output,'utf8').slice(text.length);
    assert.match(appended,/v0\.97\.0/);
    assert.match(appended,/1 passed, 1 failed, 1 skipped/);
  } finally { fs.rmSync(directory,{recursive:true,force:true}); }
});
