import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { prepareRelease } from './prepare-release.mjs';
import { assertUpgradeCI } from './ci-only.mjs';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
export const scenarios = {
  'upgrade-desktop': ['baseline seed provenance', 'preserves release data and replays startup', 'executes old and new task lifecycles after upgrade'],
  'upgrade-mobile': ['phone preserves release history and sends a follow-up after upgrade'],
};
export function assertComplete(report, project) {
  if (report.errors?.length) throw new Error(`upgrade worker errors: ${JSON.stringify(report.errors)}`);
  const specs = [];
  function visit(suite) { specs.push(...(suite.specs ?? [])); for (const nested of suite.suites ?? []) visit(nested); }
  for (const suite of report.suites ?? []) visit(suite);
  for (const title of scenarios[project]) {
    const matches = specs.filter(s => s.title === title).flatMap(s => s.tests).filter(t => t.projectName === project);
    if (matches.length !== 1 || matches[0].results?.length !== 1 || matches[0].results[0].status !== 'passed') {
      throw new Error(`required upgrade scenario did not execute exactly once and pass: ${title}`);
    }
  }
}
export function parseOptions(args) {
  const options = {baseline:'latest', engine:'sqlite', project:'upgrade-desktop', noBuild:false};
  for (let i=0;i<args.length;i++) {
    if (args[i] === '--no-build') { options.noBuild=true;continue; }
    const key = {'--baseline':'baseline','--engine':'engine','--project':'project'}[args[i]];
    if (!key || !args[i+1] || args[i+1].startsWith('--')) throw new Error(`invalid upgrade argument: ${args[i]}`);
    options[key]=args[++i];
  }
  if (options.baseline !== 'latest' && !/^v\d+\.\d+\.\d+$/.test(options.baseline)) throw new Error('baseline must be latest or a Stable tag');
  if (!['sqlite','postgres'].includes(options.engine)) throw new Error('engine must be sqlite or postgres');
  if (!scenarios[options.project]) throw new Error('unknown upgrade project');
  if (options.engine === 'postgres' && options.project === 'upgrade-mobile') throw new Error('phone smoke requires SQLite');
  return options;
}
export async function withCleanup(operation, cleanup) {
  try { return await operation(); } finally { await cleanup(); }
}
export async function runRehearsal(options) {
  assertUpgradeCI();
  const output = fs.mkdtempSync(path.join(root, '.cache/upgrade/run-'));
  const run = (command, args, config={}) => execFileSync(command, args, {cwd:root, stdio:'inherit', ...config});
  let container;
  const cleanup = () => {
    if (container) { try { run('docker',['rm','--force',container]); } finally { container=undefined; } }
  };
  const onSignal = () => { cleanup(); process.exit(130); };
  process.once('SIGTERM',onSignal);process.once('SIGINT',onSignal);
  return withCleanup(async () => {
    let manifest = process.env.KANDEV_UPGRADE_MANIFEST;
    if (!manifest) {
      const baselineDir = path.join(output,'baseline');
      prepareRelease(options.baseline,baselineDir);
      manifest=path.join(baselineDir,'manifest.json');
    }
    manifest=path.resolve(root,manifest);
    const baseline=JSON.parse(fs.readFileSync(manifest,'utf8'));
    if (options.baseline !== 'latest' && baseline.tag !== options.baseline) throw new Error('provided manifest does not match requested baseline');
    const env={...process.env, KANDEV_UPGRADE_MANIFEST:manifest, KANDEV_UPGRADE_ENGINE:options.engine, KANDEV_UPGRADE_PROJECT:options.project,
      PLAYWRIGHT_JSON_OUTPUT_FILE:path.join(output,'playwright.json'), PLAYWRIGHT_JUNIT_OUTPUT_FILE:path.join(output,'junit.xml')};
    // Candidate processes never receive GitHub authentication from acquisition.
    delete env.GH_TOKEN;delete env.GITHUB_TOKEN;
    if (options.engine === 'postgres' && !env.KANDEV_UPGRADE_POSTGRES_URL) {
      container=`kandev-upgrade-${process.pid}-${Date.now()}`;
      run('docker',['run','--detach','--rm','--name',container,'-e','POSTGRES_USER=kandev','-e','POSTGRES_PASSWORD=kandev','-e','POSTGRES_DB=postgres','-p','127.0.0.1::5432','--health-cmd','pg_isready -U kandev -d postgres','--health-interval','1s','--health-retries','60','postgres:16-alpine']);
      run('docker',['exec',container,'sh','-c','until pg_isready -U kandev -d postgres; do sleep 1; done'],{timeout:90_000,killSignal:'SIGKILL'});
      const binding=execFileSync('docker',['port',container,'5432'],{encoding:'utf8'}).trim().split('\n')[0];
      env.KANDEV_UPGRADE_POSTGRES_URL=`postgresql://kandev:kandev@${binding}/postgres`;
      env.KANDEV_UPGRADE_POSTGRES_CONTAINER=container;
    }
    const args=['e2e/scripts/run-e2e.sh','--host','--shards','1','--project',options.project];
    if (options.noBuild) args.push('--no-build');
    args.push('--','--reporter=list,json,junit');
    let failure;
    try { run('bash',args,{cwd:path.join(root,'apps/web'),env}); } catch (error) { failure=error; }
    const reportPath=path.join(output,'playwright.json');
    if (!fs.existsSync(reportPath)) throw new Error(`upgrade produced no required scenario report; see the CI job log; evidence: ${output}`, {cause:failure});
    assertComplete(JSON.parse(fs.readFileSync(reportPath,'utf8')),options.project);
    if (failure) throw failure;
    console.log(`Upgrade accepted: ${baseline.tag} -> ${execFileSync('git',['rev-parse','HEAD'],{cwd:root,encoding:'utf8'}).trim()} (${options.engine}/${options.project}); evidence: ${output}`);
  }, async () => { try { cleanup(); } finally { process.off('SIGTERM',onSignal);process.off('SIGINT',onSignal); } });
}
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    assertUpgradeCI();
    fs.mkdirSync(path.join(root,'.cache/upgrade'),{recursive:true});
    await runRehearsal(parseOptions(process.argv.slice(2)));
  } catch(error) { console.error(`upgrade failed: ${error.stack}`);process.exitCode=1; }
}
