import test from "node:test";
import assert from "node:assert/strict";

import {
  DEFAULT_CATALOGUE_PATH,
  parseStableVersion,
  parseArguments,
  readCatalogue,
  updatePins,
  formatCatalogue,
} from "./update-agent-runtime-pins.mjs";

const catalogue = {
  "@example/claude": "1.0.0",
  "opencode-ai": "1.18.32",
  "@opencode/cli": "2.0.18",
};

test("updates only existing trusted catalogue entries", () => {
  const result = updatePins(catalogue, {
    "@example/claude": "1.1.0",
    "opencode-ai": "1.18.32",
    "@opencode/cli": "2.0.18",
    "@example/untrusted": "9.9.9",
  });

  assert.equal(result.changed, true);
  assert.deepEqual(result.catalogue, {
    "@example/claude": "1.1.0",
    "opencode-ai": "1.18.32",
    "@opencode/cli": "2.0.18",
  });
});

test("does not report a change when every latest value matches", () => {
  const result = updatePins(catalogue, {
    "@example/claude": "1.0.0",
    "opencode-ai": "1.18.32",
    "@opencode/cli": "2.0.18",
  });

  assert.equal(result.changed, false);
  assert.deepEqual(result.catalogue, catalogue);
});

test("rejects prerelease, malformed, and missing latest values before rewriting", () => {
  for (const latest of ["1.1.0-beta.1", "latest", "", undefined]) {
    assert.throws(
      () => updatePins(catalogue, {
        "@example/claude": latest,
        "opencode-ai": "1.18.32",
        "@opencode/cli": "2.0.18",
      }),
      /stable SemVer|latest version/i,
    );
  }
});

test("rejects an invalid current catalogue pin", () => {
  assert.throws(
    () => updatePins({ "@example/claude": "1.0.0-beta.1" }, { "@example/claude": "1.1.0" }),
    /stable SemVer/i,
  );
});

test("rejects OpenCode package major changes in current and latest pins", () => {
  assert.throws(
    () => updatePins({ "opencode-ai": "2.0.0" }, { "opencode-ai": "2.0.0" }),
    /major/i,
  );
  assert.throws(
    () => updatePins({ "@opencode/cli": "2.0.0" }, { "@opencode/cli": "3.0.0" }),
    /major/i,
  );
});

test("accepts build metadata but not prerelease syntax", () => {
  assert.equal(parseStableVersion("1.2.3+build.7"), "1.2.3+build.7");
  assert.throws(() => parseStableVersion("1.2.3-rc.1"), /stable SemVer/i);
});

test("keeps both OpenCode packages pinned to their supported major", async () => {
  const current = await readCatalogue();
  assert.match(current["opencode-ai"], /^1\./);
  assert.match(current["@opencode/cli"], /^2\./);
  assert.throws(
    () => updatePins(current, { ...current, "@opencode/cli": "3.0.0" }),
    /major/i,
  );
});

test("parses the workflow output flag before the default catalogue path", () => {
  assert.deepEqual(parseArguments(["--github-output", "/tmp/github-output"]), {
    cataloguePath: DEFAULT_CATALOGUE_PATH,
    outputPath: "/tmp/github-output",
  });
});

test("formats the catalogue as one complete atomic-write payload", () => {
  assert.equal(
    formatCatalogue({ "@example/claude": "1.1.0", "@opencode/cli": "2.0.0" }),
    '{\n  "@example/claude": "1.1.0",\n  "@opencode/cli": "2.0.0"\n}\n',
  );
});
