import { createHash } from "node:crypto";
import { isDeepStrictEqual } from "node:util";

export const digest = (bytes) =>
  createHash("sha256").update(bytes).digest("hex");
export function assetNames(version) {
  return {
    archive: `plugin-sdk-${version}.tgz`,
    receipt: `plugin-sdk-${version}.receipt.json`,
    signature: `plugin-sdk-${version}.receipt.json.asc`,
  };
}
export function makeReceipt(source, candidate, workflow, bootstrapAllowed) {
  const receipt = {
    schema_version: 1,
    sdk_version: source.record.sdk_version,
    sdk_tag: source.sdkTag,
    source_commit: source.commit,
    host_tag: source.hostTag,
    record_digest: source.recordDigest,
    archive_integrity: candidate.integrity,
    files: candidate.files,
    node: candidate.node,
    npm: candidate.npm,
    ...workflow,
    bootstrap_allowed: bootstrapAllowed,
  };
  validateReceipt(receipt, source);
  return receipt;
}
export function validateReceipt(receipt, source) {
  const expected = {
    schema_version: 1,
    sdk_version: source.record.sdk_version,
    sdk_tag: source.sdkTag,
    source_commit: source.commit,
    host_tag: source.hostTag,
    record_digest: source.recordDigest,
    node: source.packConfig.node,
    npm: source.packConfig.npm,
  };
  for (const [key, value] of Object.entries(expected))
    if (receipt[key] !== value)
      throw new Error(`receipt ${key} differs from the original signed source`);
  if (!isDeepStrictEqual(receipt.files, [...source.packConfig.files].sort()))
    throw new Error("receipt file inventory differs from the original source");
  if (
    typeof receipt.archive_integrity !== "string" ||
    !/^sha512-[A-Za-z0-9+/]{86}==$/.test(receipt.archive_integrity)
  )
    throw new Error("receipt has invalid archive integrity");
  if (
    receipt.workflow_ref !==
      "kdlbs/kandev/.github/workflows/release.yml@refs/heads/main" ||
    !/^[0-9a-f]{40}$/.test(receipt.workflow_sha) ||
    !/^[1-9]\d*$/.test(receipt.run_id) ||
    !/^[1-9]\d*$/.test(receipt.run_attempt)
  )
    throw new Error("receipt has an invalid release workflow identity");
  if (
    typeof receipt.bootstrap_allowed !== "boolean" ||
    (receipt.bootstrap_allowed && receipt.sdk_version !== "0.1.0")
  )
    throw new Error("receipt has an invalid bootstrap policy");
}
export function verifiedFingerprint(status, approved) {
  const signatures = status
    .split("\n")
    .filter((line) => line.startsWith("[GNUPG:] VALIDSIG "));
  if (
    signatures.length !== 1 ||
    /\[GNUPG:\] (?:BADSIG|EXPKEYSIG|EXPSIG|REVKEYSIG|ERRSIG|FAILURE)\b/.test(
      status,
    )
  )
    throw new Error(
      "signature verification did not produce one valid signature",
    );
  const fields = signatures[0].split(/\s+/);
  const primary = fields[11] ?? fields[2];
  if (primary !== approved)
    throw new Error(
      "signature does not match the approved primary fingerprint",
    );
  return primary;
}
