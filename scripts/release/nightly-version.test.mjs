import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import test from "node:test";

import { nightlyVersion } from "./nightly-version.mjs";

const fullSha = "abc123def4567890abc123def4567890abc123de";

test("builds the next patch nightly version from a stable version and commit SHA", () => {
  assert.equal(
    nightlyVersion("0.82.0", fullSha),
    "0.82.1-nightly.shaabc123def456",
  );
});

test("keeps leading zeroes in the hexadecimal SHA prefix", () => {
  assert.equal(
    nightlyVersion("12.34.56", "0000000000017890abc123def4567890abc123de"),
    "12.34.57-nightly.sha000000000001",
  );
});

test("rejects non-stable or non-canonical base versions", () => {
  for (const version of [
    "v0.82.0",
    "0.82",
    "0.82.0-beta.1",
    "01.2.3",
    "1.02.3",
    "1.2.03",
  ]) {
    assert.throws(() => nightlyVersion(version, fullSha), /stable SemVer/);
  }
});

test("rejects anything except a full lowercase Git commit SHA", () => {
  for (const sha of [
    "abc123def456",
    fullSha.toUpperCase(),
    `${fullSha}0`,
    "not-a-sha",
  ]) {
    assert.throws(
      () => nightlyVersion("0.82.0", sha),
      /40-character lowercase hexadecimal/,
    );
  }
});

test("prints the nightly version when invoked as a CLI", () => {
  const output = execFileSync(process.execPath, [
    fileURLToPath(new URL("./nightly-version.mjs", import.meta.url)),
    "0.82.0",
    fullSha,
  ]);
  assert.equal(output.toString(), "0.82.1-nightly.shaabc123def456\n");
});

test("exits non-zero with usage when invoked without arguments", () => {
  const result = spawnSync(
    process.execPath,
    [fileURLToPath(new URL("./nightly-version.mjs", import.meta.url))],
    { encoding: "utf8" },
  );

  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Usage:/);
});
