import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {scenarios} from './run.mjs';

const escape = value => String(value ?? '').replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('>','&gt;')
  .replaceAll('|','&#124;').replaceAll('`','&#96;').replace(/[\r\n]+/g,' ');

export function renderSummary({manifest, candidate, engine, project, outcome, report, notes=[], workflowRef, workflowSha}) {
  const lines = [
    '## Release upgrade regression', '',
    `**Migration: ${escape(manifest?.tag ?? 'Baseline unavailable')} → ${escape(candidate || 'Candidate unavailable')}**`, '',
    `Job result: ${escape(outcome || 'unknown')}`, '',
    '| Identity | Value |', '| --- | --- |',
    `| Stable release | ${escape(manifest?.tag ?? 'Unavailable (acquisition did not complete)')} |`,
    `| Stable source commit | ${escape(manifest?.source_commit ?? 'Unavailable')} |`,
    `| Candidate commit | ${escape(candidate || 'Unavailable')} |`,
    `| Verified archive SHA256 | ${escape(manifest?.archive_sha256 ?? 'Unavailable')} |`,
  ];
  if (workflowRef) lines.push(`| Workflow definition | ${escape(workflowRef)} |`);
  if (workflowSha) lines.push(`| Workflow commit | ${escape(workflowSha)} |`);
  if (engine) lines.push(`| Database | ${escape(engine)} |`);
  if (project) lines.push(`| Browser project | ${escape(project)} |`);
  lines.push('');
  for (const note of notes) lines.push(`- ${escape(note)}`);
  if (!project) {
    lines.push('', '### Planned tests', '', '| Database | Browser project | Test |', '| --- | --- | --- |');
    for (const [database, browser] of [['SQLite','upgrade-desktop'],['PostgreSQL 16','upgrade-desktop'],['SQLite','upgrade-mobile']]) {
      for (const title of scenarios[browser]) lines.push(`| ${database} | ${browser} | ${escape(title)} |`);
    }
    lines.push('', 'This is the acquisition summary. Each matrix job reports its actual results separately.');
    return lines.join('\n') + '\n';
  }
  const specs=[];
  function visit(suite) { specs.push(...(suite.specs ?? [])); for (const child of suite.suites ?? []) visit(child); }
  for (const suite of report?.suites ?? []) visit(suite);
  const counts={};
  const rows=[];
  const titles=[...new Set([...(scenarios[project] ?? []),...specs.map(s => s.title)])];
  for (const title of titles) {
    const tests=specs.filter(s => s.title === title).flatMap(s => s.tests ?? []).filter(t => t.projectName === project);
    const results=tests.flatMap(t => t.results ?? []);
    let status=results[0]?.status ?? 'not run';
    if (tests.length > 1) status='duplicate';
    else if (results.length > 1) status='retried';
    counts[status]=(counts[status] ?? 0)+1;
    const duration=results.reduce((sum,r) => sum+(r.duration ?? 0),0)/1000;
    rows.push(`| ${escape(title)} | ${escape(status)} | ${results.length ? `${duration.toFixed(2)} s` : '—'} |`);
  }
  lines.push('', '### Test results', '', Object.entries(counts).map(([status,count]) => `${count} ${status}`).join(', '), '');
  if (!report) lines.push('No Playwright report was produced. Required scenarios are listed as not run.', '');
  if (report?.errors?.length) lines.push(`Playwright worker/report errors: ${report.errors.length}. See the job logs and artifacts.`, '');
  lines.push('| Test | Result | Duration |', '| --- | --- | --- |', ...rows, '',
    `Evidence artifact: ${escape(`upgrade-${engine}-${project}`)} (reports, runtime logs and retained traces/screenshots).`,
    'The job result also includes setup, harness checks, and runner acceptance; test rows alone do not imply an accepted upgrade.');
  return lines.join('\n') + '\n';
}

export function writeSummary(env=process.env, directory=process.cwd()) {
  if (!env.GITHUB_STEP_SUMMARY) throw new Error('GITHUB_STEP_SUMMARY is required');
  const notes=[];
  function readJSON(file, label) {
    try { return JSON.parse(fs.readFileSync(file,'utf8')); }
    catch (error) { notes.push(`${label} unavailable: ${error.code ?? error.name}`); return undefined; }
  }
  const manifest=readJSON(path.resolve(directory,env.KANDEV_UPGRADE_MANIFEST || '.cache/upgrade/baseline/manifest.json'),'Baseline manifest');
  let report;
  if (env.UPGRADE_PROJECT) {
    const cache=path.join(directory,'.cache/upgrade');
    const reports=fs.existsSync(cache) ? fs.readdirSync(cache).filter(name => name.startsWith('run-'))
      .map(name => path.join(cache,name,'playwright.json')).filter(file => fs.existsSync(file)) : [];
    if (reports.length === 1) report=readJSON(reports[0],'Playwright report');
    else notes.push(reports.length ? 'Multiple reports found; results are ambiguous.' : 'No Playwright report found.');
  }
  fs.appendFileSync(env.GITHUB_STEP_SUMMARY,renderSummary({manifest,report,notes,candidate:env.UPGRADE_CANDIDATE_SHA || env.GITHUB_SHA,
    engine:env.UPGRADE_ENGINE,project:env.UPGRADE_PROJECT,outcome:env.UPGRADE_JOB_OUTCOME,
    workflowRef:env.GITHUB_WORKFLOW_REF,workflowSha:env.GITHUB_WORKFLOW_SHA}));
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) writeSummary();
