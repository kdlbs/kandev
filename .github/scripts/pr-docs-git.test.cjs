'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const test = require('node:test');

const { GitHeadReader } = require('./pr-docs-git.cjs');

function git(cwd, ...args) {
  return execFileSync('git', args, {
    cwd,
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'pipe'],
  }).trim();
}

function writeFile(root, pathname, content) {
  const fullPath = path.join(root, pathname);
  fs.mkdirSync(path.dirname(fullPath), { recursive: true });
  fs.writeFileSync(fullPath, content);
}

function makeRemoteFixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'pr-docs-git-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));

  const remote = path.join(root, 'origin.git');
  const producer = path.join(root, 'producer');
  const checkout = path.join(root, 'checkout');
  fs.mkdirSync(producer);
  fs.mkdirSync(checkout);
  execFileSync('git', ['init', '--bare', remote], { stdio: 'ignore' });
  git(remote, 'config', 'uploadpack.allowFilter', 'true');
  execFileSync('git', ['init', producer], { stdio: 'ignore' });
  git(producer, 'config', 'user.email', 'pr-docs@example.invalid');
  git(producer, 'config', 'user.name', 'PR docs test');

  writeFile(producer, 'README.md', 'trusted base\n');
  writeFile(
    producer,
    'docs/specs/ui/requirements/unchanged.md',
    '### REQ-UI-COVERAGE-001\n### REQ-UI-COVERAGE-002\n',
  );
  writeFile(
    producer,
    'docs/specs/ui/requirements/duplicate.md',
    '### REQ-UI-COVERAGE-001\n',
  );
  writeFile(
    producer,
    'docs/specs/ui/requirements/old-name.md',
    '### REQ-UI-COVERAGE-003\n',
  );
  writeFile(
    producer,
    'docs/specs/ui/requirements/deleted.md',
    '### REQ-UI-COVERAGE-004\n',
  );
  git(producer, 'add', '.');
  git(producer, 'commit', '-m', 'trusted base');
  const trustedSha = git(producer, 'rev-parse', 'HEAD');
  git(producer, 'remote', 'add', 'origin', remote);
  git(producer, 'push', 'origin', `${trustedSha}:refs/heads/main`);

  fs.rmSync(path.join(producer, 'docs/specs/ui/requirements/old-name.md'));
  fs.rmSync(path.join(producer, 'docs/specs/ui/requirements/deleted.md'));
  writeFile(
    producer,
    'docs/specs/ui/requirements/moved/deep/new-name.md',
    '### REQ-UI-COVERAGE-003\n',
  );
  writeFile(
    producer,
    'docs/specs/ui/requirements/notes.txt',
    'REQ-UI-COVERAGE-001 appears in a non-Markdown file\n',
  );
  writeFile(
    producer,
    'docs/specs/ci/requirements/cross-system.md',
    '### REQ-UI-COVERAGE-001\n',
  );
  git(producer, 'add', '.');
  git(producer, 'commit', '-m', 'PR requirements');
  const headSha = git(producer, 'rev-parse', 'HEAD');
  git(producer, 'push', 'origin', `${headSha}:refs/pull/42/head`);

  execFileSync('git', ['init', checkout], { stdio: 'ignore' });
  git(checkout, 'remote', 'add', 'origin', remote);
  git(checkout, 'fetch', '--depth=1', 'origin', 'refs/heads/main');
  git(checkout, 'checkout', '--detach', trustedSha);
  return { checkout, headSha, trustedSha };
}

test('searches the exact fetched PR tree and returns only requirement Markdown paths', async t => {
  const fixture = makeRemoteFixture(t);
  const reader = new GitHeadReader({
    cwd: fixture.checkout,
    headSha: fixture.headSha,
    pullNumber: 42,
    trustedSha: fixture.trustedSha,
  });

  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    [
      'docs/specs/ui/requirements/duplicate.md',
      'docs/specs/ui/requirements/unchanged.md',
    ],
  );
  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-002'),
    ['docs/specs/ui/requirements/unchanged.md'],
  );
  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-003'),
    ['docs/specs/ui/requirements/moved/deep/new-name.md'],
  );
  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-004'),
    [],
  );
  assert.equal(git(fixture.checkout, 'rev-parse', 'HEAD'), fixture.trustedSha);
  assert.equal(git(fixture.checkout, 'status', '--porcelain'), '');
});

const TRUSTED_SHA = 'a'.repeat(40);
const HEAD_SHA = 'b'.repeat(40);

function stubbedReader(t, { fetchedSha = HEAD_SHA, searchResults = [] } = {}) {
  const calls = [];
  const reader = new GitHeadReader({
    cwd: '/trusted/checkout',
    headSha: HEAD_SHA,
    pullNumber: 42,
    trustedSha: TRUSTED_SHA,
    runGit: async (args, options) => {
      calls.push({ args, options });
      if (args[0] === 'rev-parse' && args.at(-1) === 'HEAD^{commit}') {
        return { code: 0, stderr: Buffer.alloc(0), stdout: Buffer.from(`${TRUSTED_SHA}\n`) };
      }
      if (args[0] === 'fetch') {
        return { code: 0, stderr: Buffer.alloc(0), stdout: Buffer.alloc(0) };
      }
      if (args[0] === 'rev-parse' && args.at(-1) === 'FETCH_HEAD^{commit}') {
        return { code: 0, stderr: Buffer.alloc(0), stdout: Buffer.from(`${fetchedSha}\n`) };
      }
      if (args[0] === 'grep') {
        return searchResults.shift() ?? {
          code: 1,
          stderr: Buffer.alloc(0),
          stdout: Buffer.alloc(0),
        };
      }
      throw new Error('unexpected Git command');
    },
  });
  t.after(() => assert.ok(calls.every(call => call.options.cwd === '/trusted/checkout')));
  return { calls, reader };
}

function grepOutput(...paths) {
  return Buffer.from(paths.map(pathname => `${HEAD_SHA}:${pathname}\0`).join(''));
}

test('caches complete no-match results and fetches each exact head once', async t => {
  const { calls, reader } = stubbedReader(t, {
    searchResults: [{ code: 1, stderr: Buffer.alloc(0), stdout: Buffer.alloc(0) }],
  });

  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    [],
  );
  assert.deepEqual(
    await reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    [],
  );
  assert.equal(calls.filter(call => call.args[0] === 'fetch').length, 1);
  assert.equal(calls.filter(call => call.args[0] === 'grep').length, 1);
  assert.deepEqual(calls.find(call => call.args[0] === 'fetch').args.slice(-2), [
    'origin',
    'refs/pull/42/head',
  ]);
});

test('rejects a fetched PR ref that does not match the API snapshot', async t => {
  const { calls, reader } = stubbedReader(t, { fetchedSha: 'c'.repeat(40) });

  await assert.rejects(
    reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    /fetched revision does not match the API snapshot/,
  );
  assert.equal(calls.some(call => call.args[0] === 'grep'), false);
});

test('treats incomplete, unsafe, and over-limit grep output as lookup errors', async t => {
  const malformedResults = [
    { code: 0, stderr: Buffer.alloc(0), stdout: Buffer.from(`${HEAD_SHA}:docs/specs/ui/requirements/a.md`) },
    { code: 0, stderr: Buffer.alloc(0), stdout: grepOutput('docs/specs/ci/requirements/a.md') },
    { code: 2, stderr: Buffer.alloc(0), stdout: Buffer.alloc(0) },
  ];
  for (const result of malformedResults) {
    await t.test(String(result.code) + ':' + result.stdout.length, async () => {
      const { reader } = stubbedReader(t, { searchResults: [result] });
      await assert.rejects(
        reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
        /Git object lookup search failed/,
      );
    });
  }

  const tooManyPaths = Array.from({ length: 201 }, (_, index) =>
    `docs/specs/ui/requirements/file-${index}.md`
  );
  const { reader } = stubbedReader(t, { searchResults: [{
    code: 0,
    stderr: Buffer.alloc(0),
    stdout: grepOutput(...tooManyPaths),
  }] });
  await assert.rejects(
    reader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    /candidate limit exceeded/,
  );
});

test('reports Git timeouts and output limits without returning partial matches', async t => {
  const timeoutReader = new GitHeadReader({
    cwd: '/trusted/checkout',
    headSha: HEAD_SHA,
    pullNumber: 42,
    trustedSha: TRUSTED_SHA,
    runGit: async () => {
      const error = new Error('process timeout');
      error.code = 'ETIMEDOUT';
      throw error;
    },
  });
  await assert.rejects(
    timeoutReader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    /time limit exceeded/,
  );

  const outputReader = new GitHeadReader({
    cwd: '/trusted/checkout',
    headSha: HEAD_SHA,
    pullNumber: 42,
    trustedSha: TRUSTED_SHA,
    runGit: async (_args, options) => ({
      code: 0,
      stderr: Buffer.alloc(0),
      stdout: Buffer.alloc(options.maxOutputBytes + 1),
    }),
  });
  await assert.rejects(
    outputReader.findRequirementPaths('docs/specs/ui/requirements', 'REQ-UI-COVERAGE-001'),
    /output limit exceeded/,
  );
});
