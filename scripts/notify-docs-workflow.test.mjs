import assert from "node:assert/strict";
import fs from "node:fs/promises";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const repoRoot = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  "..",
);
const workflowPath = path.join(
  repoRoot,
  ".github/workflows/notify-docs.yml",
);

function extractJob(workflow, name) {
  const marker = `  ${name}:\n`;
  const start = workflow.indexOf(marker);
  assert.notEqual(start, -1, `workflow job not found: ${name}`);

  const bodyStart = start + marker.length;
  const remainder = workflow.slice(bodyStart);
  const nextJob = remainder.search(/\n  [a-z0-9-]+:\n/);
  return remainder.slice(0, nextJob === -1 ? remainder.length : nextJob);
}

function extractStep(job, name) {
  const marker = `      - name: ${name}\n`;
  const start = job.indexOf(marker);
  assert.notEqual(start, -1, `workflow step not found: ${name}`);

  const bodyStart = start + marker.length;
  const remainder = job.slice(bodyStart);
  const nextStep = remainder.search(/\n      - name: /);
  return remainder.slice(0, nextStep === -1 ? remainder.length : nextStep);
}

const tokenExpression = String.raw`\$\{\{ secrets\.GITHUB_TOKEN \}\}`;

test("docs preview build uses an authenticated read-only token", async () => {
  const workflow = await fs.readFile(workflowPath, "utf8");
  const preview = extractJob(workflow, "preview");
  const build = extractStep(preview, "Build preview from pull request docs");
  const validate = extractJob(workflow, "validate");
  const testStep = extractStep(validate, "Test public docs validator");

  assert.match(
    preview,
    /permissions:\n      contents: read\n\n    steps:/,
    "preview job permissions must be limited to contents: read",
  );
  assert.match(build, new RegExp(`GITHUB_TOKEN: ${tokenExpression}`));
  assert.doesNotMatch(
    preview.replace(build, ""),
    /GITHUB_TOKEN/,
    "GITHUB_TOKEN must remain scoped to the landing build step",
  );
  assert.match(testStep, /scripts\/validate-public-docs\.test\.mjs/);
  assert.match(testStep, /scripts\/notify-docs-workflow\.test\.mjs/);
  assert.match(workflow, /- "scripts\/notify-docs-workflow\.test\.mjs"/);
});

test("preview comment publication is isolated from pull request content", async () => {
  const workflow = await fs.readFile(workflowPath, "utf8");
  const preview = extractJob(workflow, "preview");
  const publication = extractJob(workflow, "publish-preview-link");
  const publishStep = extractStep(publication, "Publish docs preview link");

  assert.match(
    preview,
    /outputs:\n      enabled: \$\{\{ steps\.cloudflare\.outputs\.enabled \}\}\n      deployment_url: \$\{\{ steps\.deploy\.outputs\.deployment-url \}\}\n      alias_url: \$\{\{ steps\.deploy\.outputs\.pages-deployment-alias-url \}\}/,
  );
  assert.match(publication, /needs: preview/);
  assert.match(
    publication,
    /if: needs\.preview\.outputs\.enabled == 'true'/,
  );
  assert.match(publication, /permissions:\n      contents: read\n      issues: write\n      pull-requests: write\n/);
  assert.match(
    publishStep,
    new RegExp(
      `DEPLOYMENT_URL: ${String.raw`\$\{\{ needs\.preview\.outputs\.deployment_url \}\}`}`,
    ),
  );
  assert.match(
    publishStep,
    new RegExp(
      `ALIAS_URL: ${String.raw`\$\{\{ needs\.preview\.outputs\.alias_url \}\}`}`,
    ),
  );
  assert.match(publishStep, /uses: actions\/github-script@/);
  assert.doesNotMatch(publication, /actions\/checkout@/);
  assert.doesNotMatch(publication, /pnpm run build:pages/);
});
