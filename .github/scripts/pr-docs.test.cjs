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

// @covers AC-CI-PR-DOCS-001.7
test('the canonical plugin registry source is exempt', () => {
  const result = validator.classifyChangedFiles([
    { filename: 'plugin-registry/plugins.yaml', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, false);
  assert.deepEqual(result.triggeringPaths, []);
  assert.deepEqual(result.exemptPaths, ['plugin-registry/plugins.yaml']);
});

// @covers AC-CI-PR-DOCS-001.7
test('the registry exemption does not cover other registry files or mixed changes', () => {
  const otherRegistryFile = validator.classifyChangedFiles([
    { filename: 'plugin-registry/schema.json', status: 'modified' },
  ]);
  assert.equal(otherRegistryFile.requiresCoverage, true);
  assert.deepEqual(otherRegistryFile.triggeringPaths, ['plugin-registry/schema.json']);

  const registryBuilder = validator.classifyChangedFiles([
    { filename: 'plugin-registry/build-index.mjs', status: 'modified' },
  ]);
  assert.equal(registryBuilder.requiresCoverage, true);
  assert.deepEqual(registryBuilder.triggeringPaths, ['plugin-registry/build-index.mjs']);

  const mixedChange = validator.classifyChangedFiles([
    { filename: 'plugin-registry/plugins.yaml', status: 'modified' },
    { filename: 'apps/backend/runtime.go', status: 'modified' },
  ]);
  assert.equal(mixedChange.requiresCoverage, true);
  assert.deepEqual(mixedChange.exemptPaths, ['plugin-registry/plugins.yaml']);
  assert.deepEqual(mixedChange.triggeringPaths, ['apps/backend/runtime.go']);
});

// @covers AC-CI-PR-DOCS-001.2
test('recognized harness configuration paths are exempt', () => {
  const result = validator.classifyChangedFiles([
    { filename: '.codex/agents/pr-poller.toml', status: 'modified' },
    { filename: '.codex/config.toml', status: 'modified' },
    { filename: '.claude/settings.json', status: 'modified' },
    { filename: '.cursor/rules/kandev-harness.mdc', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, false);
  assert.deepEqual(result.triggeringPaths, []);
  assert.deepEqual(result.exemptPaths, [
    '.codex/agents/pr-poller.toml',
    '.codex/config.toml',
    '.claude/settings.json',
    '.cursor/rules/kandev-harness.mdc',
  ]);
});

// @covers AC-CI-PR-DOCS-001.8
test('CI infrastructure paths under .github are exempt', () => {
  const result = validator.classifyChangedFiles([
    { filename: '.github/workflows/release.yml', status: 'modified' },
    { filename: '.github/scripts/pr-walkthrough-workflow-contract_test.py', status: 'modified' },
    { filename: '.github/actions/setup-opencode/action.yml', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, false);
  assert.deepEqual(result.triggeringPaths, []);
  assert.deepEqual(result.exemptPaths, [
    '.github/workflows/release.yml',
    '.github/scripts/pr-walkthrough-workflow-contract_test.py',
    '.github/actions/setup-opencode/action.yml',
  ]);
});

// @covers AC-CI-PR-DOCS-001.8
test('a CI path combined with a non-exempt path still requires coverage', () => {
  const result = validator.classifyChangedFiles([
    { filename: '.github/workflows/release.yml', status: 'modified' },
    { filename: 'apps/backend/runtime.go', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, true);
  assert.deepEqual(result.exemptPaths, ['.github/workflows/release.yml']);
  assert.deepEqual(result.triggeringPaths, ['apps/backend/runtime.go']);
});

// @covers AC-CI-PR-DOCS-001.8
test('non-CI files under .github and CI paths outside .github require coverage', () => {
  const result = validator.classifyChangedFiles([
    { filename: '.github/dependabot.yml', status: 'modified' },
    { filename: '.github/release-signing-key.asc', status: 'modified' },
    { filename: 'scripts/build.sh', status: 'modified' },
    { filename: 'workflows/ci.yml', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, true);
  assert.deepEqual(result.exemptPaths, []);
  assert.deepEqual(result.triggeringPaths, [
    '.github/dependabot.yml',
    '.github/release-signing-key.asc',
    'scripts/build.sh',
    'workflows/ci.yml',
  ]);
});

// @covers AC-CI-PR-DOCS-001.3
test('shipped files and unsupported test-like paths require coverage', () => {
  const result = validator.classifyChangedFiles([
    { filename: 'apps/web/package.json', status: 'modified' },
    { filename: 'apps/web/lib/fixture.test.json', status: 'modified' },
    { filename: 'apps/backend/worker_test.go.txt', status: 'modified' },
    { filename: 'apps/web/src/locales/en/common.yaml', status: 'modified' },
    { filename: 'apps/web/src/locales/en/nested/common.json', status: 'modified' },
    { filename: 'apps/web/src/runtime.ts', status: 'modified' },
  ]);

  assert.equal(result.requiresCoverage, true);
  assert.deepEqual(result.triggeringPaths, [
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

  const singleQuotedBackslash = validator.parseFrontmatter(
    `---\nrequirements: ['a${String.fromCharCode(92)}']\n---\n`,
  );
  assert.deepEqual(singleQuotedBackslash.data.requirements, [`a${String.fromCharCode(92)}`]);

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

function uiFixtureContents() {
  const contents = fixtureContents();
  contents['docs/plans/recovery/plan.md'] = contents['docs/plans/recovery/plan.md']
    .replaceAll('REQ-CI-PR-DOCS-001', 'REQ-UI-COVERAGE-001')
    .replaceAll(
      '../../specs/ci/system-design/pull-request-documentation-coverage.md',
      '../../specs/ui/system-design/ui-coverage.md',
    );
  contents['docs/plans/recovery/task-01-recovery.md'] = contents[
    'docs/plans/recovery/task-01-recovery.md'
  ]
    .replaceAll('REQ-CI-PR-DOCS-001', 'REQ-UI-COVERAGE-001')
    .replaceAll('AC-CI-PR-DOCS-001.3', 'AC-UI-COVERAGE-001.3')
    .replaceAll(
      '../../specs/ci/system-design/pull-request-documentation-coverage.md',
      '../../specs/ui/system-design/ui-coverage.md',
    );
  delete contents['docs/specs/ci/system-design/pull-request-documentation-coverage.md'];
  delete contents['docs/specs/ci/requirements/pull-request-documentation-coverage.md'];
  contents['docs/specs/ui/system-design/ui-coverage.md'] = `---
status: current
system: ui
requirements:
  - REQ-UI-COVERAGE-001
---

# UI coverage design
`;
  contents['docs/specs/ui/requirements/ui-coverage.md'] = `---
status: current
system: ui
---

### REQ-UI-COVERAGE-001

- **AC-UI-COVERAGE-001.3:** Runtime changes have a work order.
`;
  return contents;
}

function multiDesignFixtureContents() {
  const contents = fixtureContents();
  contents['docs/plans/recovery/plan.md'] = `---
status: draft
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-WEB-BROWSER-RETENTION-001
system_design:
  - ../../specs/platform/system-design/logging.md
  - ../../specs/web/system-design/retention.md
---

# Recovery plan

- [ ] [Task 01: Recover worktrees](task-01-recovery.md)
`;
  contents['docs/plans/recovery/task-01-recovery.md'] = `---
id: "01-recovery"
title: "Recover worktrees"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
  - REQ-WEB-BROWSER-RETENTION-001
acceptance_criteria:
  - AC-PLATFORM-DIAGNOSTIC-LOGGING-001.1
  - AC-WEB-BROWSER-RETENTION-001.1
system_design:
  - ../../specs/platform/system-design/logging.md
  - ../../specs/web/system-design/retention.md
---

# Task 01
`;
  delete contents['docs/specs/ci/system-design/pull-request-documentation-coverage.md'];
  delete contents['docs/specs/ci/requirements/pull-request-documentation-coverage.md'];
  contents['docs/specs/platform/system-design/logging.md'] = `---
status: current
system: platform
requirements:
  - REQ-PLATFORM-DIAGNOSTIC-LOGGING-001
---

# Diagnostic logging design
`;
  contents['docs/specs/web/system-design/retention.md'] = `---
status: current
system: web
requirements:
  - REQ-WEB-BROWSER-RETENTION-001
---

# Browser retention design
`;
  contents['docs/specs/platform/requirements/diagnostic-logging.md'] = `### REQ-PLATFORM-DIAGNOSTIC-LOGGING-001

- **AC-PLATFORM-DIAGNOSTIC-LOGGING-001.1:** Runtime changes have a work order.
`;
  contents['docs/specs/web/requirements/browser-retention.md'] = `### REQ-WEB-BROWSER-RETENTION-001

- **AC-WEB-BROWSER-RETENTION-001.1:** Runtime changes have a work order.
`;
  return contents;
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

test('multi-design work orders validate each requirement against its owning design', () => {
  const result = validator.validateCoverage({
    changedFiles: runtimeAndWorkOrderDiff(),
    fileContents: multiDesignFixtureContents(),
  });

  assert.equal(result.ok, true, result.errors.join('; '));
  assert.deepEqual(result.errors, []);
});

// @covers AC-CI-PR-DOCS-001.4
test('coverage loading searches only requirements declared by each design', async () => {
  const contents = multiDesignFixtureContents();
  const changed = runtimeAndWorkOrderDiff();
  const searches = [];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname, ref) {
      assert.equal(ref, SHA_B);
      if (!Object.hasOwn(contents, pathname)) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      return contents[pathname];
    },
    async searchCode(requirementId, directory) {
      searches.push({ requirementId, directory });
      return [directory.includes('/platform/')
        ? 'docs/specs/platform/requirements/diagnostic-logging.md'
        : 'docs/specs/web/requirements/browser-retention.md'];
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.deepEqual(searches, [
    {
      requirementId: 'REQ-PLATFORM-DIAGNOSTIC-LOGGING-001',
      directory: 'docs/specs/platform/requirements',
    },
    {
      requirementId: 'REQ-WEB-BROWSER-RETENTION-001',
      directory: 'docs/specs/web/requirements',
    },
  ]);
});

test('multi-design work orders fail when a requirement has no owning design', () => {
  const contents = multiDesignFixtureContents();
  contents['docs/specs/web/system-design/retention.md'] = contents[
    'docs/specs/web/system-design/retention.md'
  ].replace('  - REQ-WEB-BROWSER-RETENTION-001\n', '');

  const result = validator.validateCoverage({
    changedFiles: runtimeAndWorkOrderDiff(),
    fileContents: contents,
  });

  assert.equal(result.ok, false);
  assert.equal(
    result.errors.some(error =>
      error.includes('REQ-WEB-BROWSER-RETENTION-001')
      && error.includes('not declared by any referenced system design')
    ),
    true,
    result.errors.join('; '),
  );
});

test('linked plans and designs accept existing update metadata', () => {
  const contents = fixtureContents();
  contents['docs/plans/recovery/plan.md'] = contents['docs/plans/recovery/plan.md'].replace(
    'status: draft',
    'status: draft\nupdated: 2026-09-09',
  );
  contents['docs/specs/ci/system-design/pull-request-documentation-coverage.md'] = contents[
    'docs/specs/ci/system-design/pull-request-documentation-coverage.md'
  ].replace(
    'status: draft',
    'status: draft\nlast_updated: 2026-09-09',
  );

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
    {
      name: 'work-order basename without a Markdown link',
      overrides: {
        'docs/plans/recovery/plan.md': baseContents['docs/plans/recovery/plan.md'].replace(
          '- [ ] [Task 01: Recover worktrees](task-01-recovery.md)',
          'The task-01-recovery.md file is tracked here.',
        ),
      },
      expected: 'does not link back',
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
const SHA_D = 'd'.repeat(40);
const SHA_E = 'e'.repeat(40);

function pullRequest(number, headSha, labels = [], changedFiles = 1) {
  return {
    number,
    state: 'open',
    draft: false,
    changed_files: changedFiles,
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

test('GitHub client retries a transient response with the server delay', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls === 1) {
        return {
          ok: false,
          status: 503,
          headers: { 'retry-after': '0' },
          async text() {
            return JSON.stringify({ message: 'Service Unavailable' });
          },
        };
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            number: 42,
            head: { sha: SHA_B },
            base: { sha: SHA_A, ref: 'main' },
            labels: [],
            changed_files: 0,
          });
        },
      };
    },
  });

  const result = await client.getPullRequest(42);

  assert.equal(result.head.sha, SHA_B);
  assert.equal(calls, 2);
  assert.deepEqual(delays, [0]);
});

test('GitHub client uses the documented secondary-limit delays without headers', async () => {
  let calls = 0;
  const delays = [];
  const logs = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'secret-token',
    sleepImpl: async delay => delays.push(delay),
    logImpl: message => logs.push(message),
    fetchImpl: async () => {
      calls += 1;
      return {
        ok: false,
        status: 429,
        async text() {
          return JSON.stringify({ message: 'private response details' });
        },
      };
    },
  });

  await assert.rejects(client.getPullRequest(42), /HTTP 429/);

  assert.equal(calls, 3);
  assert.deepEqual(delays, [60_000, 120_000]);
  assert.equal(logs.some(log => log.includes('delay_ms=60000')), true);
  assert.equal(logs.some(log => log.includes('delay_ms=120000')), true);
  assert.equal(logs.every(log => !log.includes('secret-token')), true);
  assert.equal(logs.every(log => !log.includes('private response details')), true);
  assert.equal(logs.every(log => !log.includes('q=')), true);
  assert.equal(logs.some(log => log.includes('outcome=retry-exhausted')), true);
});

test('GitHub client shares the rate-limit sleep budget across requests', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls <= 2 || calls === 4) {
        return {
          ok: false,
          status: 429,
          async text() {
            return JSON.stringify({ message: 'secondary rate limit' });
          },
        };
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            number: 42,
            head: { sha: SHA_B },
            base: { sha: SHA_A, ref: 'main' },
            labels: [],
            changed_files: 0,
          });
        },
      };
    },
  });

  await client.getPullRequest(42);
  await assert.rejects(client.getPullRequest(42), /wait-budget-exhausted/);

  assert.equal(calls, 4);
  assert.deepEqual(delays, [60_000, 120_000]);
});

test('GitHub client retries rate-limited GraphQL error payloads', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls === 1) {
        return {
          ok: true,
          status: 200,
          async text() {
            return JSON.stringify({
              data: null,
              errors: [{ type: 'RATE_LIMITED', message: 'rate limit exceeded' }],
            });
          },
        };
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            data: { repository: { mergeQueue: { entries: { nodes: [] } } } },
          });
        },
      };
    },
  });

  const result = await client.graphql('query { repository { name } }', {});

  assert.deepEqual(result, { repository: { mergeQueue: { entries: { nodes: [] } } } });
  assert.equal(calls, 2);
  assert.deepEqual(delays, [60_000]);
});

test('GitHub client does not retry unrelated GraphQL errors', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            data: null,
            errors: [{ type: 'FORBIDDEN', message: 'permission denied' }],
          });
        },
      };
    },
  });

  await assert.rejects(client.graphql('query { repository { name } }', {}), /permission denied/);

  assert.equal(calls, 1);
  assert.deepEqual(delays, []);
});

test('GitHub client retries status writes with the original request payload', async () => {
  const requests = [];
  let calls = 0;
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async () => {},
    logImpl: () => {},
    fetchImpl: async (url, options) => {
      calls += 1;
      requests.push({ url, body: options.body });
      return {
        ok: calls > 1,
        status: calls > 1 ? 201 : 503,
        headers: { 'retry-after': '0' },
        async text() {
          return calls > 1 ? '{}' : JSON.stringify({ message: 'temporary failure' });
        },
      };
    },
  });

  await client.createCommitStatus(SHA_B, {
    description: 'Coverage failed',
    state: 'failure',
    targetUrl: 'https://github.com/kdlbs/kandev/actions/runs/42',
  });

  assert.equal(calls, 2);
  assert.deepEqual(requests[1], requests[0]);
});

test('GitHub client uses a due primary rate-limit reset', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls === 1) {
        return {
          ok: false,
          status: 403,
          headers: {
            'x-ratelimit-remaining': '0',
            'x-ratelimit-reset': String(Math.floor(Date.now() / 1000)),
          },
          async text() {
            return JSON.stringify({ message: 'API rate limit exceeded' });
          },
        };
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            number: 42,
            head: { sha: SHA_B },
            base: { sha: SHA_A, ref: 'main' },
            labels: [],
            changed_files: 0,
          });
        },
      };
    },
  });

  const result = await client.getPullRequest(42);

  assert.equal(result.number, 42);
  assert.equal(calls, 2);
  assert.deepEqual(delays, [0]);
});

test('GitHub client stops before an explicit wait exceeds the sleep budget', async () => {
  let calls = 0;
  const delays = [];
  const logs = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    logImpl: message => logs.push(message),
    fetchImpl: async () => {
      calls += 1;
      return {
        ok: false,
        status: 503,
        headers: { 'retry-after': '181' },
        async text() {
          return JSON.stringify({ message: 'Service Unavailable' });
        },
      };
    },
  });

  await assert.rejects(client.getPullRequest(42), /wait-budget-exhausted/);

  assert.equal(calls, 1);
  assert.deepEqual(delays, []);
  assert.equal(logs.some(log => log.includes('next_delay_ms=181000')), true);
});

test('GitHub client retries transport failures with short exponential backoff', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls < 3) {
        throw new Error('network unavailable');
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            number: 42,
            head: { sha: SHA_B },
            base: { sha: SHA_A, ref: 'main' },
            labels: [],
            changed_files: 0,
          });
        },
      };
    },
  });

  const result = await client.getPullRequest(42);

  assert.equal(result.number, 42);
  assert.equal(calls, 3);
  assert.deepEqual(delays, [250, 500]);
});

test('GitHub client retries HTTP 408 and secondary rate-limit 403 responses', async t => {
  const cases = [
    {
      name: '408',
      response: { ok: false, status: 408 },
      expectedDelay: 250,
    },
    {
      name: 'secondary 403',
      response: {
        ok: false,
        status: 403,
        async text() {
          return JSON.stringify({ message: 'You have exceeded a secondary rate limit.' });
        },
      },
      expectedDelay: 60_000,
    },
  ];

  for (const scenario of cases) {
    await t.test(scenario.name, async () => {
      let calls = 0;
      const delays = [];
      const client = new validator.GitHubClient({
        owner: 'kdlbs',
        repo: 'kandev',
        token: 'token',
        sleepImpl: async delay => delays.push(delay),
        fetchImpl: async () => {
          calls += 1;
          if (calls === 1) {
            return {
              ...scenario.response,
              async text() {
                return scenario.response.text
                  ? scenario.response.text()
                  : JSON.stringify({ message: 'Request timed out' });
              },
            };
          }
          return {
            ok: true,
            status: 200,
            async text() {
              return JSON.stringify({
                number: 42,
                head: { sha: SHA_B },
                base: { sha: SHA_A, ref: 'main' },
                labels: [],
                changed_files: 0,
              });
            },
          };
        },
      });

      const result = await client.getPullRequest(42);

      assert.equal(result.number, 42);
      assert.equal(calls, 2);
      assert.deepEqual(delays, [scenario.expectedDelay]);
    });
  }
});

test('GitHub client retries an unreadable transient body and invalid retryable JSON', async t => {
  for (const mode of ['body', 'json']) {
    await t.test(mode, async () => {
      let calls = 0;
      const client = new validator.GitHubClient({
        owner: 'kdlbs',
        repo: 'kandev',
        token: 'token',
        sleepImpl: async () => {},
        fetchImpl: async () => {
          calls += 1;
          if (calls === 1) {
            return {
              ok: false,
              status: 503,
              async text() {
                if (mode === 'body') {
                  throw new Error('response body stream closed');
                }
                return '<html>temporarily unavailable</html>';
              },
            };
          }
          return {
            ok: true,
            status: 200,
            async text() {
              return JSON.stringify({
                number: 42,
                head: { sha: SHA_B },
                base: { sha: SHA_A, ref: 'main' },
                labels: [],
                changed_files: 0,
              });
            },
          };
        },
      });

      const result = await client.getPullRequest(42);

      assert.equal(result.number, 42);
      assert.equal(calls, 2);
    });
  }
});

test('GitHub client fails permanent 4xx body and JSON errors without retry', async t => {
  for (const mode of ['body', 'json']) {
    await t.test(mode, async () => {
      let calls = 0;
      const delays = [];
      const logs = [];
      const client = new validator.GitHubClient({
        owner: 'kdlbs',
        repo: 'kandev',
        token: 'token',
        sleepImpl: async delay => delays.push(delay),
        logImpl: message => logs.push(message),
        fetchImpl: async () => {
          calls += 1;
          return {
            ok: false,
            status: 404,
            async text() {
              if (mode === 'body') {
                throw new Error('response body stream closed');
              }
              return '<html>not found</html>';
            },
          };
        },
      });

      await assert.rejects(client.getPullRequest(42), /HTTP 404/);

      assert.equal(calls, 1);
      assert.deepEqual(delays, []);
      assert.equal(logs.some(log => log.includes('status=HTTP 404')), true);
      assert.equal(logs.some(log => log.includes('attempts=1')), true);
      assert.equal(logs.some(log => log.includes('outcome=permanent')), true);
      assert.equal(logs.some(log => log.includes('request retry')), false);
    });
  }
});

test('GitHub client fails permanent responses after one attempt', async () => {
  let calls = 0;
  const delays = [];
  const logs = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    logImpl: message => logs.push(message),
    fetchImpl: async () => {
      calls += 1;
      return {
        ok: false,
        status: 404,
        async text() {
          return JSON.stringify({ message: 'Not Found' });
        },
      };
    },
  });

  await assert.rejects(client.getPullRequest(42), /HTTP 404/);

  assert.equal(calls, 1);
  assert.deepEqual(delays, []);
  assert.equal(logs.some(log => log.includes('category=http')), true);
  assert.equal(logs.some(log => log.includes('outcome=permanent')), true);
});

test('GitHub client retries request timeouts as transport failures', async () => {
  let calls = 0;
  const delays = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async delay => delays.push(delay),
    fetchImpl: async () => {
      calls += 1;
      if (calls === 1) {
        const error = new Error('request timed out');
        error.name = 'TimeoutError';
        throw error;
      }
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            number: 42,
            head: { sha: SHA_B },
            base: { sha: SHA_A, ref: 'main' },
            labels: [],
            changed_files: 0,
          });
        },
      };
    },
  });

  const result = await client.getPullRequest(42);

  assert.equal(result.number, 42);
  assert.equal(calls, 2);
  assert.deepEqual(delays, [250]);
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

test('GitHub client scopes merge-queue lookup to the target branch', async () => {
  const requests = [];
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    fetchImpl: async (url, options) => {
      requests.push({ url, options });
      return {
        ok: true,
        status: 200,
        async text() {
          return JSON.stringify({
            data: {
              repository: {
                mergeQueue: {
                  entries: {
                    nodes: [],
                    pageInfo: { hasNextPage: false, endCursor: null },
                  },
                },
              },
            },
          });
        },
      };
    },
  });

  assert.deepEqual(await client.listMergeQueueEntries('release/next'), []);
  const request = JSON.parse(requests[0].options.body);
  assert.match(request.query, /mergeQueue\(branch: \$branch\)/);
  assert.equal(request.variables.branch, 'release/next');
});

test('GitHub client converts a request timeout into a bounded error', async () => {
  let signal;
  const client = new validator.GitHubClient({
    owner: 'kdlbs',
    repo: 'kandev',
    token: 'token',
    sleepImpl: async () => {},
    logImpl: () => {},
    fetchImpl: async (_url, options) => {
      signal = options.signal;
      const error = new Error('request timed out');
      error.name = 'TimeoutError';
      throw error;
    },
  });

  await assert.rejects(client.getPullRequest(42), /timed out/i);
  assert.equal(typeof signal?.aborted, 'boolean');
});

test('coverage loading searches only the referenced requirement files', async () => {
  const contents = uiFixtureContents();
  const loaded = [];
  const searches = [];
  const workOrderPath = 'docs/plans/recovery/task-01-recovery.md';
  const changed = [
    { filename: 'apps/web/lib/runtime.ts', status: 'modified' },
    { filename: workOrderPath, status: 'modified', additions: 1, changes: 1 },
  ];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname) {
      loaded.push(pathname);
      if (!Object.hasOwn(contents, pathname)) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      return contents[pathname];
    },
    async searchCode(requirementId, directory) {
      searches.push({ requirementId, directory });
      return ['docs/specs/ui/requirements/ui-coverage.md'];
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });
  assert.equal(result.ok, true, result.errors.join('; '));
  assert.deepEqual(searches, [{
    requirementId: 'REQ-UI-COVERAGE-001',
    directory: 'docs/specs/ui/requirements',
  }]);
  assert.deepEqual(loaded, [
    workOrderPath,
    'docs/plans/recovery/plan.md',
    'docs/specs/ui/system-design/ui-coverage.md',
    'docs/specs/ui/requirements/ui-coverage.md',
  ]);
});

test('a changed requirement body with the same identity avoids code search', async () => {
  const baseContents = uiFixtureContents();
  const headContents = uiFixtureContents();
  const requirementPath = 'docs/specs/ui/requirements/ui-coverage.md';
  headContents[requirementPath] = headContents[requirementPath].replace(
    'Runtime changes have a work order.',
    'Runtime changes have an updated work order.',
  );
  const changed = [
    { filename: 'apps/web/lib/runtime.ts', status: 'modified' },
    {
      filename: 'docs/plans/recovery/task-01-recovery.md',
      status: 'modified',
      additions: 1,
      changes: 1,
    },
    { filename: requirementPath, status: 'modified', additions: 1, changes: 1 },
  ];
  const loaded = [];
  const searches = [];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname, ref) {
      loaded.push({ pathname, ref });
      if (ref === SHA_A) {
        return baseContents[pathname];
      }
      if (Object.hasOwn(headContents, pathname)) {
        return headContents[pathname];
      }
      throw new Error('GitHub API request failed with HTTP 404: Not Found');
    },
    async searchCode(requirementId, directory) {
      searches.push({ requirementId, directory });
      throw new Error('code search should not run for an unchanged identity');
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.deepEqual(searches, []);
  assert.deepEqual(
    loaded.filter(entry => entry.pathname === requirementPath),
    [
      { pathname: requirementPath, ref: SHA_B },
      { pathname: requirementPath, ref: SHA_A },
    ],
  );
});

test('a requirement rename can reuse its unchanged base identity', async () => {
  const requirementPath = 'docs/specs/ui/requirements/ui-coverage.md';
  const renamedPath = 'docs/specs/ui/requirements/renamed-coverage.md';
  const baseContents = uiFixtureContents();
  const headContents = uiFixtureContents();
  headContents[renamedPath] = headContents[requirementPath];
  delete headContents[requirementPath];
  const changed = [
    { filename: 'apps/web/lib/runtime.ts', status: 'modified' },
    {
      filename: 'docs/plans/recovery/task-01-recovery.md',
      status: 'modified',
      additions: 1,
      changes: 1,
    },
    {
      filename: renamedPath,
      previous_filename: requirementPath,
      status: 'renamed',
      additions: 1,
      deletions: 1,
      changes: 2,
    },
  ];
  const loaded = [];
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname, ref) {
      loaded.push({ pathname, ref });
      const source = ref === SHA_A ? baseContents : headContents;
      if (Object.hasOwn(source, pathname)) {
        return source[pathname];
      }
      throw new Error('GitHub API request failed with HTTP 404: Not Found');
    },
    async searchCode() {
      throw new Error('code search should not run for an unchanged rename');
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.deepEqual(
    loaded.filter(entry => entry.pathname === renamedPath || entry.pathname === requirementPath),
    [
      { pathname: renamedPath, ref: SHA_B },
      { pathname: requirementPath, ref: SHA_A },
    ],
  );
});

function repeatedCoverageFixture() {
  const contents = uiFixtureContents();
  const planPath = 'docs/plans/recovery/plan.md';
  const workOrderPath = 'docs/plans/recovery/task-01-recovery.md';
  const designPath = 'docs/specs/ui/system-design/ui-coverage.md';
  const requirementPath = 'docs/specs/ui/requirements/ui-coverage.md';
  const requirementIds = [1, 2, 3].map(index => `REQ-UI-COVERAGE-00${index}`);
  for (const pathname of [planPath, workOrderPath, designPath]) {
    contents[pathname] = contents[pathname].replace(
      '  - REQ-UI-COVERAGE-001',
      requirementIds.map(id => `  - ${id}`).join('\n'),
    );
  }
  contents[workOrderPath] = contents[workOrderPath].replace(
    '  - AC-UI-COVERAGE-001.3',
    requirementIds.map(id => `  - ${id.replace('REQ-', 'AC-')}.3`).join('\n'),
  );
  contents[requirementPath] = requirementIds.map(id =>
    `### ${id}\n\n- **${id.replace('REQ-', 'AC-')}.3:** Runtime changes have a work order.\n`
  ).join('\n');
  const changed = runtimeAndWorkOrderDiff();
  for (let index = 2; index <= 6; index += 1) {
    const basename = `task-0${index}-recovery.md`;
    const pathname = `docs/plans/recovery/${basename}`;
    contents[pathname] = contents[workOrderPath].replace('01-recovery', `0${index}-recovery`);
    contents[planPath] += `\n- [Task ${index}](${basename})\n`;
    changed.push({ filename: pathname, status: 'modified', changes: 1 });
  }
  return { contents, changed, requirementPath, requirementIds };
}

function coverageClient(contents, changed, overrides = {}) {
  return {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname, ref) {
      assert.ok([SHA_A, SHA_B].includes(ref));
      if (
        ref === SHA_A
        && changed.some(change => change?.filename === pathname && change?.status === 'added')
      ) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      if (!Object.hasOwn(contents, pathname)) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      return contents[pathname];
    },
    ...overrides,
  };
}

// @covers AC-CI-PR-DOCS-001.4, AC-CI-PR-DOCS-003.2
test('six work orders sharing three requirements stay within the search quota', async () => {
  const { contents, changed, requirementPath, requirementIds } = repeatedCoverageFixture();
  const searches = [];
  const client = coverageClient(contents, changed, {
    async searchCode(requirementId, directory) {
      searches.push({ requirementId, directory });
      if (searches.length > 10) {
        throw new Error('GitHub API request failed with HTTP 403: API rate limit exceeded');
      }
      return [requirementPath];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.equal(result.workOrders.length, 6);
  assert.deepEqual(searches, requirementIds.map(requirementId => ({
    requirementId,
    directory: 'docs/specs/ui/requirements',
  })));
});

// @covers AC-CI-PR-DOCS-001.4, AC-CI-PR-DOCS-003.2
test('newly added requirements resolve from the head diff without code search', async () => {
  const { contents, changed, requirementPath: originalPath } = repeatedCoverageFixture();
  const requirementPath = 'docs/specs/ui/requirements/coverage.md';
  contents[requirementPath] = contents[originalPath];
  delete contents[originalPath];
  changed.push({ filename: requirementPath, status: 'added' });
  let searches = 0;
  let listings = 0;
  const client = coverageClient(contents, changed, {
    async searchCode() {
      searches += 1;
      throw new Error('code search should not run for a newly added requirement');
    },
    async listDirectory(directory, ref) {
      assert.equal(directory, 'docs/specs/ui/requirements');
      assert.equal(ref, SHA_B);
      listings += 1;
      return [{ path: requirementPath, type: 'file' }];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.equal(searches, 0);
  assert.equal(listings, 1);
});

// @covers AC-CI-PR-DOCS-001.5
test('newly added canonical requirements still reject duplicate IDs', async () => {
  const { contents, changed, requirementPath: originalPath } = repeatedCoverageFixture();
  const requirementPath = 'docs/specs/ui/requirements/coverage.md';
  const duplicatePath = 'docs/specs/ui/requirements/duplicate.md';
  contents[requirementPath] = contents[originalPath];
  delete contents[originalPath];
  contents[duplicatePath] = contents[requirementPath];
  changed.push({ filename: requirementPath, status: 'added' });
  let searches = 0;
  let listings = 0;
  const client = coverageClient(contents, changed, {
    async searchCode() {
      searches += 1;
      return [requirementPath];
    },
    async listDirectory(directory, ref) {
      assert.equal(directory, 'docs/specs/ui/requirements');
      assert.equal(ref, SHA_B);
      listings += 1;
      return [
        { path: requirementPath, type: 'file' },
        { path: duplicatePath, type: 'file' },
      ];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'invalid', result.errors.join('; '));
  assert.match(result.errors.join('; '), /ambiguous definitions/);
  assert.equal(searches, 0);
  assert.equal(listings, 1);
});

// @covers AC-CI-PR-DOCS-004.3
test('rate-limited code search scans the exact-head requirements directory', async () => {
  const { contents, changed, requirementPath } = repeatedCoverageFixture();
  changed.push({ filename: requirementPath, status: 'added' });
  let searches = 0;
  let listings = 0;
  const client = coverageClient(contents, changed, {
    async searchCode() {
      searches += 1;
      throw new Error(
        'GitHub API request stopped: class=code-search category=rate-limit '
        + 'status=HTTP 429 attempts=3 outcome=wait-budget-exhausted next_delay_ms=736000',
      );
    },
    async listDirectory(directory, ref) {
      assert.equal(directory, 'docs/specs/ui/requirements');
      assert.equal(ref, SHA_B);
      listings += 1;
      return [{ path: requirementPath, type: 'file' }];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.equal(searches, 1);
  assert.equal(listings, 1);
  assert.equal(result.acceptedReferences.length, result.workOrders.length);
});

// @covers AC-CI-PR-DOCS-004.3
test('secondary-rate-limited code search uses the directory scan', async () => {
  const { contents, changed, requirementPath } = repeatedCoverageFixture();
  changed.push({ filename: requirementPath, status: 'added' });
  let listings = 0;
  const client = coverageClient(contents, changed, {
    async searchCode() {
      throw new Error(
        'GitHub API request stopped: class=code-search category=rate-limit '
        + 'status=HTTP 403 attempts=3 outcome=wait-budget-exhausted next_delay_ms=736000',
      );
    },
    async listDirectory() {
      listings += 1;
      return [{ path: requirementPath, type: 'file' }];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.equal(listings, 1);
});

// @covers AC-CI-PR-DOCS-004.3
test('rate-limit directory scan still rejects duplicate requirement definitions', async () => {
  const { contents, changed, requirementPath } = repeatedCoverageFixture();
  const duplicatePath = 'docs/specs/ui/requirements/duplicate.md';
  contents[duplicatePath] = contents[requirementPath];
  changed.push({ filename: requirementPath, status: 'added' });
  const client = coverageClient(contents, changed, {
    async searchCode() {
      throw new Error(
        'GitHub API request stopped: class=code-search category=rate-limit '
        + 'status=HTTP 429 attempts=3 outcome=wait-budget-exhausted next_delay_ms=736000',
      );
    },
    async listDirectory() {
      return [requirementPath, duplicatePath].map(path => ({ path, type: 'file' }));
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'invalid');
  assert.match(result.errors.join('; '), /ambiguous definitions/);
});

// @covers AC-CI-PR-DOCS-004.3
test('rate-limit directory scan fails closed when its document bound is exceeded', async () => {
  const { contents, changed } = repeatedCoverageFixture();
  const directoryEntries = Array.from({ length: 201 }, (_, index) => ({
    path: `docs/specs/ui/requirements/candidate-${index}.md`,
    type: 'file',
  }));
  const client = coverageClient(contents, changed, {
    async searchCode() {
      throw new Error(
        'GitHub API request stopped: class=code-search category=rate-limit '
        + 'status=HTTP 429 attempts=3 outcome=wait-budget-exhausted next_delay_ms=736000',
      );
    },
    async listDirectory() {
      return directoryEntries;
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'error');
  assert.match(result.errors.join('; '), /directory scan exceeds the 200-document limit/);
});

// @covers AC-CI-PR-DOCS-004.3
test('rate-limit directory scan fails closed when the listing is unavailable', async () => {
  const { contents, changed, requirementPath } = repeatedCoverageFixture();
  changed.push({ filename: requirementPath, status: 'added' });
  const client = coverageClient(contents, changed, {
    async searchCode() {
      throw new Error(
        'GitHub API request stopped: class=code-search category=rate-limit '
        + 'status=HTTP 429 attempts=3 outcome=wait-budget-exhausted next_delay_ms=736000',
      );
    },
    async listDirectory() {
      throw new Error('GitHub API request failed with HTTP 404: Not Found');
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'error');
  assert.match(result.errors.join('; '), /requirements directory listing is missing/i);
});

// @covers AC-CI-PR-DOCS-001.4, AC-CI-PR-DOCS-001.5
test('directory fallback reuses exact-head entries without accepting missing requirements', async t => {
  for (const outcome of ['file', 'empty', 'missing']) {
    await t.test(outcome, async () => {
      const { contents, changed, requirementPath } = repeatedCoverageFixture();
      const fallbackPath = 'docs/specs/ui/requirements/coverage.md';
      contents[fallbackPath] = contents[requirementPath];
      delete contents[requirementPath];
      let listings = 0;
      const client = coverageClient(contents, changed, {
        async searchCode() {
          return [];
        },
        async listDirectory(directory, ref) {
          assert.equal(directory, 'docs/specs/ui/requirements');
          assert.equal(ref, SHA_B);
          listings += 1;
          if (outcome === 'missing') {
            throw new Error('GitHub API request failed with HTTP 404: Not Found');
          }
          return outcome === 'file' ? [{ path: fallbackPath, type: 'file' }] : [];
        },
      });

      const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

      assert.equal(result.status, outcome === 'file' ? 'covered' : 'invalid', result.errors.join('; '));
      assert.equal(listings, 1);
    });
  }
});

// @covers AC-CI-PR-DOCS-001.4, AC-CI-PR-DOCS-001.5
test('designs sharing an ID reuse lookups only within their requirement directory', async () => {
  const contents = uiFixtureContents();
  const reference = '  - ../../specs/ui/system-design/ui-coverage.md';
  for (const pathname of ['docs/plans/recovery/plan.md', 'docs/plans/recovery/task-01-recovery.md']) {
    contents[pathname] = contents[pathname].replace(reference, `${reference}
  - ../../specs/ui/system-design/shared.md
  - ../../specs/ci/system-design/shared.md`);
  }
  contents['docs/specs/ui/system-design/shared.md'] = contents['docs/specs/ui/system-design/ui-coverage.md'];
  contents['docs/specs/ci/system-design/shared.md'] = contents['docs/specs/ui/system-design/ui-coverage.md']
    .replace('system: ui', 'system: ci');
  contents['docs/specs/ci/requirements/shared.md'] = contents['docs/specs/ui/requirements/ui-coverage.md'];
  const searches = [];
  const client = coverageClient(contents, runtimeAndWorkOrderDiff(), {
    async searchCode(requirementId, directory) {
      searches.push({ requirementId, directory });
      return [directory === 'docs/specs/ui/requirements'
        ? 'docs/specs/ui/requirements/ui-coverage.md'
        : 'docs/specs/ci/requirements/shared.md'];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.deepEqual(searches, ['ui', 'ci'].map(system => ({
    requirementId: 'REQ-UI-COVERAGE-001',
    directory: `docs/specs/${system}/requirements`,
  })));
});

// @covers AC-CI-PR-DOCS-003.2
test('lookup errors fail closed without poisoning later evaluations', async t => {
  for (const boundary of ['searchCode', 'listDirectory']) {
    await t.test(boundary, async () => {
      const { contents, changed, requirementPath } = repeatedCoverageFixture();
      const directoryFallbackPath = 'docs/specs/ui/requirements/coverage.md';
      contents[directoryFallbackPath] = contents[requirementPath];
      let fail = true;
      const client = coverageClient(contents, changed, {
        async searchCode() {
          if (fail && boundary === 'searchCode') {
            throw new Error('GitHub API request failed with HTTP 403: API rate limit exceeded');
          }
          return boundary === 'listDirectory' ? [] : [requirementPath];
        },
        async listDirectory() {
          if (fail && boundary === 'listDirectory') {
            throw new Error('GitHub API request failed with HTTP 503: Service Unavailable');
          }
          return [{ path: directoryFallbackPath, type: 'file' }];
        },
      });

      const failed = await validator.evaluatePullRequest({ client, pullNumber: 42 });
      assert.equal(failed.ok, false);
      assert.equal(failed.status, 'error');
      assert.match(failed.errors.join('; '), /GitHub API request failed/);
      fail = false;
      const recovered = await validator.evaluatePullRequest({ client, pullNumber: 42 });
      assert.equal(recovered.status, 'covered', recovered.errors.join('; '));
    });
  }
});

// @covers AC-CI-PR-DOCS-001.5
test('lookup reuse preserves ambiguous requirement detection', async t => {
  for (const source of ['search', 'PR-only file']) {
    await t.test(source, async () => {
      const { contents, changed, requirementPath } = repeatedCoverageFixture();
      const duplicatePath = 'docs/specs/ui/requirements/duplicate.md';
      contents[duplicatePath] = contents[requirementPath];
      if (source === 'PR-only file') {
        changed.push({ filename: duplicatePath, status: 'added' });
      }
      const client = coverageClient(contents, changed, {
        async searchCode() {
          return source === 'search' ? [requirementPath, duplicatePath] : [requirementPath];
        },
      });

      const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

      assert.equal(result.status, 'invalid');
      assert.match(result.errors.join('; '), /ambiguous definitions/);
    });
  }
});

// @covers AC-CI-PR-DOCS-003.1
test('revision retries reload requirement lookups and files at the new exact head', async () => {
  const { contents, changed, requirementPath, requirementIds } = repeatedCoverageFixture();
  const renamedPath = 'docs/specs/ui/requirements/renamed.md';
  let metadataReads = 0;
  let searches = 0;
  const client = coverageClient(contents, changed, {
    async getPullRequest() {
      return pullRequest(42, metadataReads++ === 0 ? SHA_B : SHA_C, [], changed.length);
    },
    async searchCode() {
      searches += 1;
      return [metadataReads === 1 ? requirementPath : renamedPath];
    },
    async getFile(pathname, ref) {
      assert.equal(ref, metadataReads === 1 ? SHA_B : SHA_C);
      if (ref === SHA_C && pathname === requirementPath) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      return contents[pathname === renamedPath ? requirementPath : pathname];
    },
  });

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });

  assert.equal(result.status, 'covered', result.errors.join('; '));
  assert.equal(result.headSha, SHA_C);
  assert.equal(searches, requirementIds.length * 2);
});

// @covers AC-CI-PR-DOCS-003.4
test('merge-group members do not share cached requirement lookups or contents', async () => {
  const { contents, changed, requirementPath } = repeatedCoverageFixture();
  let currentMember;
  const client = coverageClient(contents, changed, {
    async getPullRequest(number) {
      currentMember = number;
      return pullRequest(number, number === 1 ? SHA_D : SHA_E, [], changed.length);
    },
    async searchCode() {
      return currentMember === 1 ? [requirementPath] : [];
    },
    async getFile(pathname, ref) {
      assert.equal(ref, currentMember === 1 ? SHA_D : SHA_E);
      if (currentMember === 2 && pathname === requirementPath) {
        throw new Error('GitHub API request failed with HTTP 404: Not Found');
      }
      return contents[pathname];
    },
  });

  const result = await validator.evaluateMergeGroup({
    client,
    baseSha: SHA_A,
    headSha: SHA_C,
    entries: [
      { baseCommit: { oid: SHA_A }, headCommit: { oid: SHA_B }, pullRequest: { number: 1, headRefOid: SHA_D } },
      { baseCommit: { oid: SHA_B }, headCommit: { oid: SHA_C }, pullRequest: { number: 2, headRefOid: SHA_E } },
    ],
  });

  assert.equal(result.ok, false);
  assert.deepEqual(result.memberResults.map(member => member.status), ['covered', 'invalid']);
  assert.match(result.memberResults[1].errors.join('; '), /not defined in ui requirements/);
});

test('missing referenced artifacts become invalid coverage through the API adapter', async () => {
  const contents = fixtureContents();
  const workOrderPath = 'docs/plans/recovery/task-01-recovery.md';
  const changed = runtimeAndWorkOrderDiff();
  const client = {
    async getPullRequest() {
      return pullRequest(42, SHA_B, [], changed.length);
    },
    async listFiles() {
      return changed;
    },
    async getFile(pathname) {
      if (pathname === workOrderPath) {
        return contents[pathname];
      }
      throw new Error('GitHub API request failed with HTTP 404: Not Found');
    },
  };

  const result = await validator.evaluatePullRequest({ client, pullNumber: 42 });
  assert.equal(result.ok, false);
  assert.equal(result.status, 'invalid');
  assert.equal(result.errors.some(error => error.includes('plan.md')), true);
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
      pullRequest: { number: 1, headRefOid: SHA_D },
    },
    {
      baseCommit: { oid: SHA_B },
      headCommit: { oid: SHA_C },
      pullRequest: { number: 2, headRefOid: SHA_E },
    },
  ];
  const client = {
    async getPullRequest(number) {
      return number === 1
        ? pullRequest(1, SHA_D, ['no-docs-allow'])
        : pullRequest(2, SHA_E);
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
  assert.deepEqual(
    result.memberResults.map(member => member.expectedHeadSha),
    [SHA_D, SHA_E],
  );
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

test('label reevaluation includes every queued prefix containing the changed first member', () => {
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

  const groups = validator.findAffectedMergeGroups({ entries, pullRequestNumber: 1 });

  assert.deepEqual(
    groups.map(group => ({
      baseSha: group.baseSha,
      headSha: group.headSha,
      members: group.entries.map(entry => entry.pullRequest.number),
    })),
    [
      { baseSha: SHA_A, headSha: SHA_B, members: [1] },
      { baseSha: SHA_A, headSha: SHA_C, members: [1, 2] },
    ],
  );
});

test('label removal reevaluates both prefix and full groups for the first queued member', async () => {
  const statuses = [];
  const entries = [
    {
      baseCommit: { oid: SHA_A },
      headCommit: { oid: SHA_B },
      pullRequest: { number: 42, headRefOid: SHA_B },
    },
    {
      baseCommit: { oid: SHA_B },
      headCommit: { oid: SHA_C },
      pullRequest: { number: 43, headRefOid: SHA_C },
    },
  ];
  const client = {
    async getPullRequest(number) {
      return number === 42 ? pullRequest(42, SHA_B) : pullRequest(43, SHA_C);
    },
    async listFiles(number) {
      return [{
        filename: number === 42 ? 'apps/backend/runtime.go' : 'docs/guide.md',
        status: 'modified',
      }];
    },
    async listMergeQueueEntries(branch) {
      assert.equal(branch, 'main');
      return entries;
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, state: status.state });
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
  assert.deepEqual(result.result.affectedGroups.map(group => group.headSha), [SHA_B, SHA_C]);
  assert.deepEqual(statuses.map(status => status.sha), [SHA_B, SHA_B, SHA_B, SHA_B, SHA_C, SHA_C]);
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

test('run reuses the snapshot used for the pending status', async () => {
  let metadataCalls = 0;
  const statuses = [];
  const client = {
    async getPullRequest() {
      metadataCalls += 1;
      return pullRequest(42, SHA_B);
    },
    async listFiles() {
      return [{ filename: 'docs/guide.md', status: 'modified' }];
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, state: status.state });
    },
  };

  const result = await validator.run({
    client,
    env: {},
    event: { pull_request: { number: 42 } },
    eventName: 'pull_request_target',
    writeSummary: () => {},
  });

  assert.equal(result.exitCode, 0);
  assert.equal(metadataCalls, 2);
  assert.deepEqual(statuses.map(status => status.state), ['pending', 'success']);
});

test('workflow dispatch reads the pull-request number from its input', async () => {
  const statuses = [];
  const client = {
    async getPullRequest(number) {
      assert.equal(number, 99);
      return pullRequest(99, SHA_B);
    },
    async listFiles() {
      return [{ filename: 'docs/guide.md', status: 'modified' }];
    },
    async createCommitStatus(sha, status) {
      statuses.push({ sha, state: status.state });
    },
  };

  const result = await validator.run({
    client,
    env: {
      GITHUB_REPOSITORY: 'kdlbs/kandev',
      GITHUB_RUN_ID: '100',
      GITHUB_SERVER_URL: 'https://github.com',
    },
    event: { inputs: { pr_number: '99' } },
    eventName: 'workflow_dispatch',
    writeSummary: () => {},
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(statuses, [
    { sha: SHA_B, state: 'pending' },
    { sha: SHA_B, state: 'success' },
  ]);
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
    event: {
      merge_group: {
        base_ref: 'refs/heads/main',
        base_sha: SHA_A,
        head_sha: SHA_C,
      },
    },
    eventName: 'merge_group',
    writeSummary: () => {},
  });

  assert.equal(result.exitCode, 0);
  assert.deepEqual(statuses.map(status => status.state), ['pending', 'success']);
  assert.deepEqual(statuses.map(status => status.sha), [SHA_C, SHA_C]);
  assert.equal(statuses[1].description, 'Every merge-group member satisfies documentation coverage');
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
