import assert from "node:assert/strict";
import test from "node:test";
import {
  makeReceipt,
  validateReceipt,
  verifiedFingerprint,
} from "./sdk-artifact.mjs";

const source = {
  commit: "a".repeat(40),
  hostTag: "v0.100.0",
  sdkTag: "pluginsdk/v0.1.0",
  recordDigest: "b".repeat(64),
  record: { sdk_version: "0.1.0" },
  packConfig: {
    node: "24.19.0",
    npm: "11.21.0",
    files: [
      "LICENSE",
      "README.md",
      "package.json",
      "sdk-release.json",
      "src/index.ts",
    ],
  },
};
const candidate = {
  integrity: "sha512-" + "A".repeat(86) + "==",
  files: source.packConfig.files,
  node: "24.19.0",
  npm: "11.21.0",
};
const workflow = {
  workflow_ref: "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main",
  workflow_sha: "c".repeat(40),
  run_id: "123456",
  run_attempt: "1",
};
test("receipt binds original source, metadata, archive, tools and workflow identities", () => {
  const receipt = makeReceipt(source, candidate, workflow, true);
  assert.equal(receipt.source_commit, source.commit);
  assert.equal(receipt.workflow_sha, workflow.workflow_sha);
  assert.equal(receipt.archive_integrity, candidate.integrity);
  validateReceipt(receipt, source);
  for (const patch of [
    { source_commit: "d".repeat(40) },
    { record_digest: "e".repeat(64) },
    { host_tag: "v0.101.0" },
    { node: "24.18.0" },
    { files: ["package.json"] },
    { workflow_ref: "evil/repo/release.yml@refs/heads/main" },
    { run_id: "unknown" },
  ]) {
    assert.throws(
      () => validateReceipt({ ...receipt, ...patch }, source),
      /receipt/,
    );
  }
});
test("signature validation requires the approved primary fingerprint", () => {
  const primary = "A".repeat(40),
    subkey = "B".repeat(40);
  const status = `[GNUPG:] VALIDSIG ${subkey} 2026-10-19 123 0 4 0 22 8 00 ${primary}\n`;
  assert.equal(verifiedFingerprint(status, primary), primary);
  assert.throws(() => verifiedFingerprint(status, "C".repeat(40)), /signature/);
  assert.throws(
    () => verifiedFingerprint("[GNUPG:] GOODSIG untrusted key\n", primary),
    /signature/,
  );
  assert.throws(
    () => verifiedFingerprint(status + status, primary),
    /signature/,
  );
});
