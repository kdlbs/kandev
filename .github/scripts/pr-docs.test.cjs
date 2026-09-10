'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const MODULE_PATH = path.join(__dirname, 'pr-docs.cjs');
const validator = require(MODULE_PATH);

test('documentation coverage validator exposes the path and artifact APIs', () => {
  assert.equal(fs.existsSync(MODULE_PATH), true);
  assert.equal(typeof validator.classifyChangedFiles, 'function');
  assert.equal(typeof validator.validateCoverage, 'function');
  assert.equal(typeof validator.parseFrontmatter, 'function');
  assert.equal(typeof validator.evaluatePullRequest, 'function');
  assert.equal(typeof validator.GitHubClient, 'function');
  assert.equal(typeof validator.resolveMergeGroupMembers, 'function');
  assert.equal(typeof validator.findAffectedMergeGroups, 'function');
  assert.equal(typeof validator.run, 'function');
});

// @covers AC-CI-PR-DOCS-001.2
test('recognized documentation-only paths are exempt', () => {
  const result = validator.classifyChangedFiles([
    { filename: 'docs/guide.txt', status: 'modified' },
    { filename: 'README.md', status: 'modified' },
    { filename: 'notes/guide.markdown', status: 'modified' },
    { filename: 'apps/backend/worker_test.go', status: 'modified' },
    { filename: 'apps/web/lib/worker.test.ts', status: 'modified' },
    { filename: 'apps/web/lib/worker.spec.jsx', status: 'modified' },
    { filename: 'apps/web/e2e/tasks/example.ts', status: 'modified' },
    { filename: 'apps/web/src/locales/en/common.json', status: 'modified' },
    { filename: 'vendor/pnpm-lock.yaml', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, false);
  assert.deepEqual(result.triggeringPaths, []);
  assert.equal(result.exemptPaths.length, 9);
});

// @covers AC-CI-PR-DOCS-001.3
test('shipped files and unsupported test-like paths require coverage', () => {
  const result = validator.classifyChangedFiles([
    { filename: '.github/workflows/release.yml', status: 'modified' },
    { filename: 'apps/web/package.json', status: 'modified' },
    { filename: 'apps/web/lib/fixture.test.json', status: 'modified' },
    { filename: 'apps/backend/worker_test.go.txt', status: 'modified' },
    { filename: 'apps/web/src/locales/en/common.yaml', status: 'modified' },
    { filename: 'apps/web/src/locales/en/nested/common.json', status: 'modified' },
    { filename: 'apps/web/src/runtime.ts', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, true);
  assert.deepEqual(result.triggeringPaths, [
    '.github/workflows/release.yml',
    'apps/web/package.json',
    'apps/web/lib/fixture.test.json',
    'apps/backend/worker_test.go.txt',
    'apps/web/src/locales/en/common.yaml',
    'apps/web/src/locales/en/nested/common.json',
    'apps/web/src/runtime.ts',
  ]);
});

test('malformed changed-file records fail closed', () => {
  const result = validator.classifyChangedFiles([{}]);
  assert.equal(result.requiresCoverage, true);
  assert.deepEqual(result.invalidPaths, ['[missing filename]']);
  assert.equal(result.reasons[0].reason, 'invalid changed-file record');
});

test('renames classify both old and new paths and pure work-order renames do not qualify', () => {
  const result = validator.classifyChangedFiles([
    {
      filename: 'docs/runtime-recovery.md',
      previous_filename: 'apps/backend/runtime-recovery.go',
      status: 'renamed',
      additions: 0,
      deletions: 0,
      changes: 0,
    },
  ]);
  assert.deepEqual(result.changedPaths, [
    'docs/runtime-recovery.md',
    'apps/backend/runtime-recovery.go',
  ]);
  assert.deepEqual(result.triggeringPaths, ['apps/backend/runtime-recovery.go']);

  assert.deepEqual(
    validator.selectChangedWorkOrders([
      {
        filename: 'docs/plans/recovery/task-01-recovery.md',
        previous_filename: 'docs/plans/old/task-01-recovery.md',
        status: 'renamed',
        additions: 0,
        deletions: 0,
        changes: 0,
      },
    ]),
    [],
  );
  assert.deepEqual(
    validator.selectChangedWorkOrders([
      {
        filename: 'docs/plans/recovery/task-01-recovery.md',
        previous_filename: 'docs/plans/old/task-01-recovery.md',
        status: 'renamed',
        additions: 1,
        deletions: 1,
        changes: 2,
      },
    ]),
    ['docs/plans/recovery/task-01-recovery.md'],
  );
});

test('frontmatter parser accepts the documented subset and rejects unsafe syntax', () => {
  const parsed = validator.parseFrontmatter(`---
id: "01-recovery"
title: "Recover worktrees"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
acceptance_criteria:
  - AC-CI-PR-DOCS-001.3
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Work order
`);
  assert.deepEqual(parsed.data.requirements, ['REQ-CI-PR-DOCS-001']);
  assert.equal(parsed.data.wave, 1);

  for (const source of [
    '---\nunknown: value\n---\n',
    '---\nid: first\nid: second\n---\n',
    '---\nrequirements: !!js/function evil\n---\n',
    'not-frontmatter',
  ]) {
    assert.throws(() => validator.parseFrontmatter(source));
  }
});

function fixtureContents(overrides = {}) {
  const files = {
    'docs/plans/recovery/plan.md': `---
status: draft
requirements:
  - REQ-CI-PR-DOCS-001
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Recovery plan

- [ ] [Task 01: Recover worktrees](task-01-recovery.md)
`,
    'docs/plans/recovery/task-01-recovery.md': `---
id: "01-recovery"
title: "Recover worktrees"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-PR-DOCS-001
acceptance_criteria:
  - AC-CI-PR-DOCS-001.3
system_design:
  - ../../specs/ci/system-design/pull-request-documentation-coverage.md
---

# Task 01

## Scope

- \`apps/backend/runtime-recovery.go\`
`,
    'docs/specs/ci/system-design/pull-request-documentation-coverage.md': `---
status: draft
system: ci
requirements:
  - REQ-CI-PR-DOCS-001
---

# Design

REQ-CI-PR-DOCS-001
`,
    'docs/specs/ci/requirements/pull-request-documentation-coverage.md': `---
status: draft
system: ci
---

### REQ-CI-PR-DOCS-001

- **AC-CI-PR-DOCS-001.3:** Runtime changes have a work order.
`,
    'apps/backend/runtime-recovery.go': 'package runtime\n',
  };
  return { ...files, ...overrides };
}

function runtimeAndWorkOrderDiff(overrides = []) {
  return [
    { filename: 'apps/backend/runtime-recovery.go', status: 'modified' },
    {
      filename: 'docs/plans/recovery/task-01-recovery.md',
      status: 'modified',
      additions: 3,
      changes: 3,
    },
    ...overrides,
  ];
}

// @covers AC-CI-PR-DOCS-001.3, AC-CI-PR-DOCS-001.4
test('valid linked work order covers a runtime change without editing existing contracts', () => {
  const result = validator.validateCoverage({
    changedFiles: runtimeAndWorkOrderDiff(),
    fileContents: fixtureContents(),
  });

  assert.equal(result.ok, true);
  assert.equal(result.status, 'covered');
  assert.deepEqual(result.workOrders, ['docs/plans/recovery/task-01-recovery.md']);
  assert.deepEqual(result.errors, []);
});

test('multi-requirement work orders map acceptance criteria to their owning requirement', () => {
  const contents = fixtureContents();
  contents['docs/plans/recovery/plan.md'] = contents['docs/plans/recovery/plan.md'].replace(
    '  - REQ-CI-PR-DOCS-001\nsystem_design:',
    '  - REQ-CI-PR-DOCS-001\n  - REQ-CI-PR-DOCS-002\nsystem_design:',
  );
  contents['docs/plans/recovery/task-01-recovery.md'] = contents[
    'docs/plans/recovery/task-01-recovery.md'
  ].replace(
    '  - REQ-CI-PR-DOCS-001\nacceptance_criteria:\n  - AC-CI-PR-DOCS-001.3',
    '  - REQ-CI-PR-DOCS-001\n  - REQ-CI-PR-DOCS-002\nacceptance_criteria:\n  - AC-CI-PR-DOCS-001.3\n  - AC-CI-PR-DOCS-002.1',
  );
  contents['docs/specs/ci/system-design/pull-request-documentation-coverage.md'] = contents[
    'docs/specs/ci/system-design/pull-request-documentation-coverage.md'
  ].replace(
    '  - REQ-CI-PR-DOCS-001\n---',
    '  - REQ-CI-PR-DOCS-001\n  - REQ-CI-PR-DOCS-002\n---',
  );
  contents['docs/specs/ci/requirements/pull-request-documentation-coverage.md'] += `

### REQ-CI-PR-DOCS-002: Explicit documentation exception

- **AC-CI-PR-DOCS-002.1:** The exact exception label passes coverage.
`;

  const result = validator.validateCoverage({
    changedFiles: runtimeAndWorkOrderDiff(),
    fileContents: contents,
  });

  assert.equal(result.ok, true, result.errors.join('; '));
  assert.deepEqual(result.errors, []);
});

// @covers AC-CI-PR-DOCS-001.5, AC-CI-PR-DOCS-001.6
test('missing, empty, deleted, escaping, and mismatched references fail precisely', () => {
  const baseContents = fixtureContents();
  const cases = [
    {
      name: 'missing plan',
      overrides: { 'docs/plans/recovery/plan.md': undefined },
      expected: 'plan.md',
    },
    {
      name: 'empty work order',
      overrides: { 'docs/plans/recovery/task-01-recovery.md': '' },
      expected: 'empty',
    },
    {
      name: 'missing dependency list',
      overrides: {
        'docs/plans/recovery/task-01-recovery.md': baseContents[
          'docs/plans/recovery/task-01-recovery.md'
        ].replace('depends_on: []\n', ''),
      },
      expected: 'depends_on',
    },
    {
      name: 'deleted design',
      overrides: {
        'docs/specs/ci/system-design/pull-request-documentation-coverage.md': undefined,
      },
      expected: 'system-design',
    },
    {
      name: 'escaping plan reference',
      overrides: {
        'docs/plans/recovery/task-01-recovery.md': baseContents[
          'docs/plans/recovery/task-01-recovery.md'
        ].replace('plan: "plan.md"', 'plan: "../../outside.md"'),
      },
      expected: 'outside.md',
    },
    {
      name: 'acceptance criterion belongs to another requirement',
      overrides: {
        'docs/specs/ci/requirements/pull-request-documentation-coverage.md': baseContents[
          'docs/specs/ci/requirements/pull-request-documentation-coverage.md'
        ].replace('REQ-CI-PR-DOCS-001', 'REQ-CI-PR-DOCS-999'),
      },
      expected: 'REQ-CI-PR-DOCS-001',
    },
  ];

  for (const { name, overrides, expected } of cases) {
    const result = validator.validateCoverage({
      changedFiles: runtimeAndWorkOrderDiff(),
      fileContents: fixtureContents(overrides),
    });
    assert.equal(result.ok, false, name);
    assert.equal(
      result.errors.some(error => error.includes(expected)),
      true,
      `${name}: ${result.errors.join('; ')}`,
    );
  }
});

test('ambiguous requirement definitions and missing work orders fail closed', () => {
  const ambiguous = validator.validateCoverage({
    changedFiles: runtimeAndWorkOrderDiff(),
    fileContents: fixtureContents({
      'docs/specs/ci/requirements/duplicate.md':
        '### REQ-CI-PR-DOCS-001\n\n- **AC-CI-PR-DOCS-001.3:** duplicate\n',
    }),
  });
  assert.equal(ambiguous.ok, false);
  assert.equal(ambiguous.errors.some(error => error.includes('ambiguous')), true);

  const noWorkOrder = validator.validateCoverage({
    changedFiles: [{ filename: 'apps/backend/runtime-recovery.go', status: 'modified' }],
    fileContents: fixtureContents(),
  });
  assert.equal(noWorkOrder.ok, false);
  assert.equal(noWorkOrder.status, 'missing');
  assert.equal(noWorkOrder.errors.some(error => error.includes('work order')), true);
});

const SHA_A = 'a'.repeat(40);
const SHA_B = 'b'.repeat(40);
const SHA_C = 'c'.repeat(40);

function pullRequest(number, headSha, labels = []) {
  return {
    number,
    state: 'open',
    draft: false,
    changed_files: 1,
    head: { sha: headSha },
    base: { sha: SHA_A, ref: 'main' },
    labels: labels.map(name => ({ name })),
  };
}

// @covers AC-CI-PR-DOCS-002.1, AC-CI-PR-DOCS-002.4
test('exact no-docs-allow label passes before changed-file reads', async () => {
  let listFilesCalls = 0;
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, ['no-docs-allow']);
    },
    async listFiles() {
      listFilesCalls += 1;
      throw new Error('changed files should not be read for an override');
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });
  assert.equal(result.ok, true);
  assert.equal(result.status, 'override');
  assert.equal(result.override, 'no-docs-allow');
  assert.equal(listFilesCalls, 0);
});

test('documentation-only changes do not load delivery artifacts', async () => {
  let getFileCalls = 0;
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B);
    },
    async listFiles() {
      return [{
        filename: 'docs/plans/recovery/task-01-recovery.md',
        status: 'modified',
      }];
    },
    async getFile() {
      getFileCalls += 1;
      throw new Error('documentation-only changes should not load artifacts');
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });
  assert.equal(result.ok, true);
  assert.equal(result.status, 'exempt');
  assert.equal(getFileCalls, 0);
});

// @covers AC-CI-PR-DOCS-002.2, AC-CI-PR-DOCS-002.3
test('label removal is reevaluated and does not leave an obsolete override', async () => {
  const metadata = [pullRequest(42, SHA_B, ['no-docs-allow']), pullRequest(42, SHA_B)];
  let metadataCalls = 0;
  const client = {
    async getPullRequest() {
      return metadata[Math.min(metadataCalls++, metadata.length - 1)];
    },
    async listFiles() {
      return [{ filename: 'apps/backend/runtime.go', status: 'modified' }];
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });
  assert.equal(result.ok, false);
  assert.equal(result.status, 'missing');
  assert.equal(metadataCalls >= 2, true);
});

// @covers AC-CI-PR-DOCS-003.1, AC-CI-PR-DOCS-003.2
test('head changes during evaluation fail closed after bounded retries', async () => {
  let metadataCalls = 0;
  const client = {
    async getPullRequest() {
      const sha = metadataCalls++ % 2 === 0 ? SHA_B : SHA_C;
      return pullRequest(42, sha);
    },
    async listFiles() {
      return [{ filename: 'apps/backend/runtime.go', status: 'modified' }];
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42, maxAttempts: 2 });
  assert.equal(result.ok, false);
  assert.equal(result.status, 'error');
  assert.equal(result.errors.some(error => error.includes('changed during evaluation')), true);
  assert.equal(metadataCalls, 4);
});

test('GitHub client rejects incomplete pages and decodes bounded file contents', async () => {
  const requests = [];
  const responses = [
    {
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify([
          { filename: 'apps/backend/runtime.go', status: 'modified' },
        ]);
      },
    },
    {
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify({
          path: 'docs/plan.md',
          type: 'file',
          encoding: 'base64',
          content: Buffer.from('hello').toString('base64'),
        });
      },
    },
  ];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token-is-not-logged',
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return responses.shift();
    },
  });

  const files = await client.listFiles(42, 1);
  assert.equal(files.length, 1);
  assert.equal(await client.getFile('docs/plan.md', SHA_B), 'hello');
  assert.equal(requests[1].options.method, 'GET');
  assert.equal(requests[1].options.headers.Authorization, 'Bearer token-is-not-logged');
});

test('GitHub client rejects malformed changed-file entries and mismatched content paths', async () => {
  const responses = [
    {
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify([{}]);
      },
    },
    {
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify({
          path: 'docs/other.md',
          type: 'file',
          encoding: 'base64',
          content: Buffer.from('hello').toString('base64'),
        });
      },
    },
  ];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    fetchImpl: async () => responses.shift(),
  });

  await assert.rejects(client.listFiles(42, 1), /filename/);
  await assert.rejects(client.getFile('docs/plan.md', SHA_B), /returned path/);
});

test('GitHub client rejects a changed-file count at the API cap', async () => {
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    fetchImpl: async () => ({
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify(Array.from({ length: 100 }, (_, index) => ({
          filename: `apps/runtime-${index}.go`,
          status: 'modified',
        })));
      },
    }),
  });

  await assert.rejects(
    client.listFiles(42, 3000),
    /3,000-file limit/,
  );
});

test('GitHub pull-request metadata requires a bounded changed-file count', async () => {
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    fetchImpl: async () => ({
      ok: true,
      status: 200,
      async text() {
        return JSON.stringify({
          number: 42,
          head: { sha: SHA_B },
          base: { sha: SHA_A },
          labels: [],
          changed_files: 3001,
        });
      },
    }),
  });

  await assert.rejects(client.getPullRequest(42), /changed-file count/);
});

// @covers AC-CI-PR-DOCS-003.4
test('merge-group members are resolved from exact entry boundaries', () => {
  const entries = [
    {
      baseCommit: { oid: SHA_A },
      headCommit: { oid: SHA_B },
      pullRequest: { number: 1, headRefOid: SHA_B },
    },
    {
      baseCommit: { oid: SHA_B },
      headCommit: { oid: SHA_C },
      pullRequest: { number: 2, headRefOid: SHA_C },
    },
  ];
  const result = validator.resolveMergeGroupMembers({
    baseSha: SHA_A,
    headSha: SHA_C,
    entries,
  });
  assert.deepEqual(result.members.map(member => member.number), [1, 2]);
  assert.equal(result.baseSha, SHA_A);
  assert.equal(result.headSha, SHA_C);

  assert.throws(
    () => validator.resolveMergeGroupMembers({
      baseSha: SHA_A,
      headSha: SHA_C,
      entries: [...entries, { ...entries[1] }],
    }),
    /ambiguous/,
  );
});

// @covers AC-CI-PR-DOCS-003.4
test('merge-group evaluation keeps each member policy independent', async () => {
  const entries = [
    {
      baseCommit: { oid: SHA_A },
      headCommit: { oid: SHA_B },
      pullRequest: { number: 1, headRefOid: SHA_B },
    },
    {
      baseCommit: { oid: SHA_B },
      headCommit: { oid: SHA_C },
      pullRequest: { number: 2, headRefOid: SHA_C },
    },
  ];
  const client = {
    async getPullRequest(number) {
      return number === 1
        ? pullRequest(1, SHA_B, ['no-docs-allow'])
        : pullRequest(2, SHA_C);
    },
    async listFiles(number) {
      return [{
        filename: `apps/backend/member-${number}.go`,
        status: 'modified',
      }];
    },
  };

  const result = await validator.evaluateMergeGroup({
    client,
    baseSha: SHA_A,
    headSha: SHA_C,
    entries,
  });
  assert.equal(result.ok, false);
  assert.deepEqual(result.memberResults.map(member => member.status), ['override', 'missing']);
});

test('affected merge groups can be found for label-triggered reevaluation', () => {
  const entries = [
    {
      baseCommit: { oid: SHA_A },
      headCommit: { oid: SHA_B },
      pullRequest: { number: 1, headRefOid: SHA_B },
    },
    {
      baseCommit: { oid: SHA_B },
      headCommit: { oid: SHA_C },
      pullRequest: { number: 2, headRefOid: SHA_C },
    },
    {
      baseCommit: { oid: SHA_A },
      headCommit: { oid: 'd'.repeat(40) },
      pullRequest: { number: 3, headRefOid: 'd'.repeat(40) },
    },
  ];
  const groups = validator.findAffectedMergeGroups({ entries, pullRequestNumber: 2 });
  assert.equal(groups.length, 1);
  assert.equal(groups[0].baseSha, SHA_A);
  assert.equal(groups[0].headSha, SHA_C);
  assert.deepEqual(groups[0].entries.map(entry => entry.pullRequest.number), [1, 2]);
});

// @covers AC-CI-PR-DOCS-001.1, AC-CI-PR-DOCS-003.1
test('run publishes pending and final status for the stable current pull-request head', async () => {
  const statuses = [];
  const summaries = [];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B);
    },
    async listFiles() {
      return [{ filename: 'docs/guide.md', status: 'modified' }];
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, ...status });
    },
  };

  const result = await validator.run({
    client,
    env: {
      GITHUB_REPOSITORY: 'kdlbs/kandev',
      GITHUB_RUN_ID: '99',
      GITHUB_SERVER_URL: 'https://github.com',
    },
    event: { pull_request: { number: 42 } },
    eventName: 'pull_request_target',
    writeSummary: summary => summaries.push(summary),
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(statuses.map(status => status.state), ['pending', 'success']);
  assert.deepEqual(statuses.map(status => status.sha), [SHA_B, SHA_B]);
  assert.equal(summaries.length, 1);
  assert.match(summaries[0], /docs\/guide\.md/);
});

test('merge-group runs publish one status on the synthetic group head', async () => {
  const statuses = [];
  const client = {
    async listMergeQueueEntries() {
      return [{
        baseCommit: { oid: SHA_A },
        headCommit: { oid: SHA_C },
        pullRequest: { number: 1, headRefOid: SHA_C },
      }];
    },
    async getPullRequest() {
      return pullRequest(1, SHA_C);
    },
    async listFiles() {
      return [{ filename: 'docs/guide.md', status: 'modified' }];
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, ...status });
    },
  };

  const result = await validator.run({
    client,
    env: {},
    event: { merge_group: { base_sha: SHA_A, head_sha: SHA_C } },
    eventName: 'merge_group',
    writeSummary: () => {},
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(statuses.map(status => status.state), ['pending', 'success']);
  assert.deepEqual(statuses.map(status => status.sha), [SHA_C, SHA_C]);
});

// @covers AC-CI-PR-DOCS-002.2, AC-CI-PR-DOCS-003.4
test('label-triggered runs reevaluate affected merge groups independently', async () => {
  const statuses = [];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_C);
    },
    async listFiles() {
      return [{ filename: 'apps/backend/runtime.go', status: 'modified' }];
    },
    async listMergeQueueEntries() {
      return [{
        baseCommit: { oid: SHA_A },
        headCommit: { oid: SHA_C },
        pullRequest: { number: 42, headRefOid: SHA_C },
      }];
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, ...status });
    },
  };

  const result = await validator.run({
    client,
    env: {},
    event: { action: 'unlabeled', pull_request: { number: 42 } },
    eventName: 'pull_request_target',
    writeSummary: () => {},
  });

  assert.equal(result.exitCode, 1);
  assert.equal(result.result.affectedGroups.length, 1);
  assert.deepEqual(statuses.map(status => status.state), [
    'pending',
    'failure',
    'pending',
    'failure',
  ]);
});

test('run summaries escape untrusted paths and error text', () => {
  const summary = validator.resultSummary({
    changedPaths: ['bad`path\n- forged item'],
    errors: ['bad <input>'],
    ok: false,
    status: 'invalid',
    triggeringPaths: ['bad`path\n- forged item'],
  });
  assert.equal(summary.includes('bad`path'), false);
  assert.equal(summary.includes('&#96;'), true);
  assert.equal(summary.includes('&lt;input&gt;'), true);
  assert.equal(summary.includes('\n- forged item'), false);
});
