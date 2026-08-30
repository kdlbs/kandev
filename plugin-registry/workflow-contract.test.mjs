import assert from "node:assert/strict";
import fs from "node:fs/promises";
import test from "node:test";

const indexPath = new URL(
  "../.github/workflows/plugin-registry-index.yml",
  import.meta.url,
);
const pollPath = new URL(
  "../.github/workflows/plugin-registry-release-poll.yml",
  import.meta.url,
);
const e2ePath = new URL("../.github/workflows/e2e-tests.yml", import.meta.url);

test("release poll is off-boundary every five minutes and never accepts repository dispatch", async () => {
  const poll = await fs.readFile(pollPath, "utf8");

  assert.match(poll, /cron:\s*["']3-58\/5 \* \* \* \*["']/);
  assert.doesNotMatch(poll, /repository_dispatch/);
  assert.match(poll, /permissions:\s*\n\s+contents:\s*read/);
  assert.match(poll, /run:\s*node plugin-registry\/check-releases\.mjs/);
});

test("release poll conditionally calls the central index workflow with deployment permissions", async () => {
  const poll = await fs.readFile(pollPath, "utf8");

  assert.match(poll, /if:\s*needs\.detect\.outputs\.rebuild == 'true'/);
  assert.match(
    poll,
    /uses:\s*\.\/\.github\/workflows\/plugin-registry-index\.yml/,
  );
  assert.match(poll, /pages:\s*write/);
  assert.match(poll, /id-token:\s*write/);
});

test("index workflow preserves daily fallback and shared serialized concurrency", async () => {
  const index = await fs.readFile(indexPath, "utf8");

  assert.match(index, /cron:\s*["']0 6 \* \* \*["']/);
  assert.match(index, /workflow_call:/);
  assert.match(index, /group:\s*plugin-registry-pages/);
  assert.match(index, /cancel-in-progress:\s*false/);
  assert.match(index, /if:\s*github\.event_name != 'pull_request'/);
});

test("index workflow verifies packages and fetches prior catalog before publication", async () => {
  const index = await fs.readFile(indexPath, "utf8");

  assert.match(index, /go build .*plugin-package-verify/);
  assert.match(
    index,
    /https:\/\/kdlbs\.github\.io\/kandev\/plugins\/index\.json/,
  );
  assert.match(index, /PLUGIN_REGISTRY_PRIOR_INDEX:/);
  assert.match(index, /PLUGIN_PACKAGE_VERIFIER:/);
  assert.match(index, /node --test plugin-registry\/\*\.test\.mjs/);
  assert.match(index, /if ! curl[\s\S]*printf 'null\\n'/);
});

test("sharded plugin E2E jobs receive an executable package verifier", async () => {
  const e2e = await fs.readFile(e2ePath, "utf8");

  assert.match(
    e2e,
    /name: e2e-plugin-package[\s\S]*path: \|[\s\S]*kandev-plugin-e2e-1\.0\.0\.tar\.gz[\s\S]*plugin-package-verify/,
  );
  assert.equal(
    e2e.match(/chmod \+x apps\/backend\/\.build\/plugin-package-verify/g)
      ?.length,
    2,
  );
});

test("Pages and OIDC permission live on deployment-capable jobs, not the default token", async () => {
  const index = await fs.readFile(indexPath, "utf8");
  const beforeJobs = index.slice(0, index.indexOf("jobs:"));

  assert.match(beforeJobs, /permissions:\s*\n\s+contents:\s*read/);
  assert.doesNotMatch(beforeJobs, /pages:\s*write/);
  assert.doesNotMatch(beforeJobs, /id-token:\s*write/);
  assert.match(
    index,
    /deploy:[\s\S]*permissions:[\s\S]*pages:\s*write[\s\S]*id-token:\s*write/,
  );
});
