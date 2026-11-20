"use strict";

const assert = require("node:assert/strict");
const test = require("node:test");

const evidence = require("./pr-ui-evidence.cjs");

const OLD_SHA = "a".repeat(40);
const NEW_SHA = "b".repeat(40);

function pullRequest({
  headSha = OLD_SHA,
  body = "",
  labels = [],
  changedFiles = 1,
} = {}) {
  return {
    number: 7,
    body,
    changed_files: changedFiles,
    labels: labels.map((name) => ({ name })),
    base: { ref: "main", sha: "c".repeat(40) },
    head: { sha: headSha },
  };
}

function uiFile(filename = "apps/web/src/components/example.tsx") {
  return { filename, status: "modified" };
}

class FakeClient {
  constructor(snapshots, files = [uiFile()]) {
    this.snapshots = [...snapshots];
    this.files = files;
    this.readCounts = [];
    this.statuses = [];
  }

  async getPullRequest(number) {
    assert.equal(number, 7);
    if (this.snapshots.length > 1) {
      return this.snapshots.shift();
    }
    return this.snapshots[0];
  }

  async listFiles(number, expectedCount) {
    assert.equal(number, 7);
    this.readCounts.push(expectedCount);
    assert.equal(this.files.length, expectedCount);
    return this.files;
  }

  async createCommitStatus(sha, status, context) {
    this.statuses.push({ sha, status, context });
  }
}

async function run(client, overrides = {}) {
  return evidence.run({
    client,
    event: { pull_request: { number: 7 } },
    eventName: "pull_request_target",
    env: {
      GITHUB_REPOSITORY: "kdlbs/kandev",
      GITHUB_SERVER_URL: "https://github.com",
      GITHUB_RUN_ID: "123",
    },
    writeSummary: async () => {},
    writeLog: () => {},
    ...overrides,
  });
}

// @covers AC-CI-UI-SCREENSHOTS-001.1, AC-CI-UI-SCREENSHOTS-001.2
test("classifies runtime UI files and both sides of a rename", () => {
  const result = evidence.classifyUIChanges([
    uiFile(),
    { filename: "apps/packages/ui/src/button.tsx", status: "added" },
    { filename: "apps/desktop/src/panel.css", status: "modified" },
    { filename: "apps/desktop/index.html", status: "modified" },
    {
      filename: "apps/web/src/other.tsx",
      previous_filename: "apps/web/src/original.tsx",
      status: "renamed",
    },
  ]);
  assert.deepEqual(result.candidatePaths, [
    "apps/web/src/components/example.tsx",
    "apps/packages/ui/src/button.tsx",
    "apps/desktop/src/panel.css",
    "apps/desktop/index.html",
    "apps/web/src/other.tsx",
    "apps/web/src/original.tsx",
  ]);
});

// @covers AC-CI-UI-SCREENSHOTS-001.1, AC-CI-UI-SCREENSHOTS-001.2
test("classifies JSX runtime components as UI candidates", () => {
  const result = evidence.classifyUIChanges([
    uiFile("apps/web/src/components/example.jsx"),
  ]);
  assert.deepEqual(result.candidatePaths, [
    "apps/web/src/components/example.jsx",
  ]);
  assert.equal(result.requiresEvidence, true);
});

// @covers AC-CI-UI-SCREENSHOTS-001.2
test("excludes tests, docs, scripts, lockfiles, translation catalogs, and tool configuration", () => {
  const paths = [
    "apps/web/src/components/example.test.tsx",
    "apps/web/e2e/tasks/example.spec.ts",
    "apps/web/src/docs/readme.md",
    "apps/web/src/scripts/build.js",
    "apps/web/pnpm-lock.yaml",
    "apps/web/src/locales/pt-pt/common.json",
    "apps/web/vite.config.ts",
    "apps/packages/ui/src/button.stories.tsx",
  ];
  const result = evidence.classifyUIChanges(
    paths.map((filename) => ({ filename, status: "modified" })),
  );
  assert.deepEqual(result.candidatePaths, []);
  assert.deepEqual(result.excludedPaths, paths);
  assert.equal(result.requiresEvidence, false);
});

// @covers AC-CI-UI-SCREENSHOTS-001.2
test("excludes locale catalogs without exempting runtime i18n code", () => {
  const paths = [
    "apps/web/lib/i18n/provider.tsx",
    "apps/web/lib/i18n/formats.ts",
    "apps/web/lib/i18n/provider.test.tsx",
    "apps/web/src/locales/pt-pt/common.json",
    "apps/web/src/locales/pt-pt/messages.po",
    "apps/web/src/locales/pt-pt/messages.pot",
  ];
  const result = evidence.classifyUIChanges(
    paths.map((filename) => ({ filename, status: "modified" })),
  );
  assert.deepEqual(result.candidatePaths, [
    "apps/web/lib/i18n/provider.tsx",
    "apps/web/lib/i18n/formats.ts",
  ]);
  assert.deepEqual(result.excludedPaths, paths.slice(2));
  assert.equal(result.requiresEvidence, true);
});

// @covers AC-CI-UI-SCREENSHOTS-001.3, AC-CI-UI-SCREENSHOTS-001.4
test("requires a descriptive HTTPS image in the Screenshots section only", () => {
  const valid = `![Task dialog on mobile](https://github.com/user-attachments/assets/abc123 "Phone")`;
  assert.equal(evidence.hasScreenshotEmbed(`## Screenshots\n\n${valid}`), true);
  assert.equal(
    evidence.hasScreenshotEmbed(
      `![badge](https://example.com/badge.svg)\n\n## Screenshots\n\nNo image here.`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n![local](apps/web/dialog.png)`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n![plain link](http://example.com/a.png)`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n![](https://example.com/a.png)`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n[image](https://example.com/a.png)`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n\\![escaped](https://example.com/a.png)`,
    ),
    false,
  );
  assert.equal(
    evidence.hasScreenshotEmbed(
      `## Screenshots\n\n    ![indented code](https://example.com/a.png)`,
    ),
    false,
  );
});

// @covers AC-CI-UI-SCREENSHOTS-001.5
test("ignores fenced examples, HTML comments, and content after the next second-level heading", () => {
  const body = [
    "## Screenshots",
    "",
    "```markdown",
    "![example](https://example.com/example.png)",
    "```",
    "<!-- ![comment](https://example.com/comment.png) -->",
    "",
    "## Validation",
    "![later section](https://example.com/later.png)",
  ].join("\n");
  assert.equal(evidence.hasScreenshotEmbed(body), false);
});

// @covers AC-CI-UI-SCREENSHOTS-001.4
test("ignores multiline code spans but accepts a visible image outside them", () => {
  const hidden = [
    "## Screenshots",
    "",
    "`Example:",
    "![Dialog](https://example.com/dialog.png)",
    "`",
  ].join("\n");
  assert.equal(evidence.hasScreenshotEmbed(hidden), false);

  const visible = [
    "## Screenshots",
    "",
    "`Example:",
    "![Hidden dialog](https://example.com/hidden.png)",
    "`",
    "![Visible dialog](https://example.com/visible.png)",
  ].join("\n");
  assert.equal(evidence.hasScreenshotEmbed(visible), true);
});

// @covers AC-CI-UI-SCREENSHOTS-001.1, AC-CI-UI-SCREENSHOTS-001.6
test("non-UI PRs pass and UI PRs without an embed fail with a useful result", async () => {
  const nonUI = new FakeClient(
    [pullRequest({ changedFiles: 1 })],
    [{ filename: "apps/backend/main.go", status: "modified" }],
  );
  const nonUIResult = await run(nonUI);
  assert.equal(nonUIResult.exitCode, 0);
  assert.equal(nonUIResult.result.status, "no_ui_changes");
  assert.equal(nonUI.statuses[0].status.state, "success");

  const ui = new FakeClient([
    pullRequest({ body: "A badge: ![build](https://example.com/ci.svg)" }),
  ]);
  const uiResult = await run(ui);
  assert.equal(uiResult.exitCode, 1);
  assert.equal(uiResult.result.status, "screenshots_missing");
  assert.match(uiResult.result.message, /## Screenshots/);
  assert.equal(ui.statuses[0].status.state, "failure");
});

// @covers AC-CI-UI-SCREENSHOTS-001.7
test("names the candidate paths that require screenshot evidence", async () => {
  const pathname = "apps/web/src/components/example.jsx";
  const ui = new FakeClient(
    [pullRequest()],
    [{ filename: pathname, status: "modified" }],
  );
  const summaries = [];
  const result = await run(ui, {
    writeSummary: async (summary) => summaries.push(summary),
  });
  assert.equal(result.result.status, "screenshots_missing");
  assert.match(summaries.join("\n"), /apps\/web\/src\/components\/example\\\.jsx/);
});

// @covers AC-CI-UI-SCREENSHOTS-001.3, AC-CI-UI-SCREENSHOTS-002.1, AC-CI-UI-SCREENSHOTS-002.2
test("an exact maintainer label passes, and removing it restores screenshot evaluation", async () => {
  const withException = new FakeClient([
    pullRequest({ labels: ["no-screenshots-allow"] }),
  ]);
  const exceptionResult = await run(withException);
  assert.equal(exceptionResult.exitCode, 0);
  assert.equal(exceptionResult.result.status, "maintainer_exception");
  assert.match(exceptionResult.result.message, /no-screenshots-allow/);

  const lookalike = new FakeClient([
    pullRequest({ labels: ["no-screenshots-allow-me"] }),
  ]);
  const lookalikeResult = await run(lookalike);
  assert.equal(lookalikeResult.exitCode, 1);
  assert.equal(lookalikeResult.result.status, "screenshots_missing");

  const removed = new FakeClient([pullRequest({ labels: [] })]);
  const removedResult = await run(removed);
  assert.equal(removedResult.exitCode, 1);
  assert.equal(removedResult.result.status, "screenshots_missing");
});

// @covers AC-CI-UI-SCREENSHOTS-002.3
test("fork pull requests follow the same screenshot rule", async () => {
  const fork = pullRequest();
  fork.head.repo = { full_name: "contributor/kandev" };
  const client = new FakeClient([fork]);
  const result = await run(client);
  assert.equal(result.exitCode, 1);
  assert.equal(result.result.status, "screenshots_missing");
  assert.equal(client.statuses[0].sha, OLD_SHA);
});

// @covers AC-CI-UI-SCREENSHOTS-003.1, AC-CI-UI-SCREENSHOTS-003.2, AC-CI-UI-SCREENSHOTS-003.3
test("reads the complete change list, rechecks metadata, retries stale snapshots, and publishes to the current head", async () => {
  const oldPR = pullRequest({ body: "", headSha: OLD_SHA });
  const newPR = pullRequest({
    body: "## Screenshots\n\n![Updated dialog](https://github.com/user-attachments/assets/new)",
    headSha: NEW_SHA,
  });
  const changed = [uiFile()];
  const client = new FakeClient([oldPR, newPR, newPR, newPR], changed);
  const result = await run(client);
  assert.equal(result.exitCode, 0);
  assert.equal(result.result.status, "screenshots_present");
  assert.deepEqual(client.readCounts, [1, 1]);
  assert.equal(client.statuses.length, 1);
  assert.equal(client.statuses[0].sha, NEW_SHA);
  assert.equal(client.statuses[0].context, "UI screenshots");
});

// @covers AC-CI-UI-SCREENSHOTS-003.3
test("returns an error when GitHub omits part of the changed-file list", async () => {
  const client = new FakeClient([pullRequest({ changedFiles: 2 })], [uiFile()]);
  client.listFiles = async () => [];
  const result = await run(client);
  assert.equal(result.exitCode, 1);
  assert.equal(result.result.status, "error");
  assert.equal(client.statuses[0].status.state, "error");
  assert.equal(client.statuses[0].sha, OLD_SHA);
});
