import { compareVersions, validateRecord } from "./plugin-sdk.mjs";

function validateContext({ source, fingerprint }) {
  validateRecord(source.record);
  if (
    !/^[0-9a-f]{40}$/.test(source.commit) ||
    !/^[A-F0-9]{40}$/.test(fingerprint)
  )
    throw new Error("invalid source commit or approved signing fingerprint");
  if (
    source.hostTag !== `v${source.record.host_release}` ||
    source.sdkTag !== `pluginsdk/v${source.record.sdk_version}`
  )
    throw new Error("source tag identity conflict");
}
function verifyTag(tag, { source, fingerprint }) {
  if (
    tag &&
    (tag.commit !== source.commit ||
      tag.fingerprint !== fingerprint ||
      tag.recordDigest !== source.recordDigest)
  )
    throw new Error("SDK tag source or signature conflict");
}
function verifyCandidate(candidate, { source, fingerprint }) {
  const r = candidate.receipt;
  if (candidate.fingerprint !== fingerprint)
    throw new Error("candidate receipt signature conflict");
  if (
    r.schema_version !== 1 ||
    r.source_commit !== source.commit ||
    r.host_tag !== source.hostTag ||
    r.sdk_tag !== source.sdkTag ||
    r.sdk_version !== source.record.sdk_version ||
    r.record_digest !== source.recordDigest ||
    r.archive_integrity !== candidate.integrity
  )
    throw new Error("candidate receipt identity conflict");
}
function verifyRegistry(registry, candidate, source) {
  if (
    registry.version &&
    (registry.version.integrity !== candidate.integrity ||
      registry.version.recordDigest !== source.recordDigest)
  )
    throw new Error("npm version integrity or record conflict");
}
async function readState(io, context) {
  const tag = await io.readTag(context.source);
  const registry = await io.readRegistry(context.source.record.sdk_version);
  verifyTag(tag, context);
  return { tag, registry };
}
async function observe(io, read, valid, label, writeError) {
  for (let attempt = 0; attempt < 3; attempt++) {
    const value = await read();
    if (valid(value)) return value;
    if (attempt < 2) await io.wait(attempt);
  }
  throw new Error(
    `${label} did not become visible${writeError ? `: ${writeError.message}` : ""}`,
  );
}

export async function preparePublication(io, context) {
  validateContext(context);
  const { source } = context;
  let { tag, registry } = await readState(io, context);
  const candidate = await io.obtainCandidate(source, {
    allowCreate: !tag && !registry.version,
    bootstrapAllowed:
      !registry.packageExists && source.record.change_kind === "initial",
  });
  verifyCandidate(candidate, context);
  verifyRegistry(registry, candidate, source);
  await io.retainCandidate(candidate, source);
  ({ tag, registry } = await readState(io, context));
  verifyRegistry(registry, candidate, source);
  if (!tag) {
    let writeError;
    try {
      await io.createTag(source);
    } catch (error) {
      writeError = error;
    }
    tag = await observe(
      io,
      () => io.readTag(source),
      (value) => {
        verifyTag(value, context);
        return value !== null;
      },
      "signed SDK tag",
      writeError,
    );
  }
  return { status: "prepared", candidate, source };
}

async function ensureLatest(io, context) {
  const version = context.source.record.sdk_version;
  let registry = await io.readRegistry(version);
  if (registry.latest === version) return;
  if (registry.latest && compareVersions(registry.latest, version) > 0) {
    await io.verifyNewerLatest(registry.latest);
    return;
  }
  let writeError;
  try {
    await io.moveLatest(version);
  } catch (error) {
    writeError = error;
  }
  registry = await observe(
    io,
    () => io.readRegistry(version),
    (state) =>
      state.latest === version ||
      (state.latest && compareVersions(state.latest, version) > 0),
    "npm latest dist-tag",
    writeError,
  );
  if (registry.latest !== version) await io.verifyNewerLatest(registry.latest);
}

export async function publishPublication(io, context) {
  validateContext(context);
  const { source } = context;
  let { tag, registry } = await readState(io, context);
  if (!tag)
    throw new Error("signed SDK tag is missing; run SDK preparation first");
  const candidate = await io.obtainCandidate(source, {
    allowCreate: false,
    bootstrapAllowed: false,
  });
  verifyCandidate(candidate, context);
  verifyRegistry(registry, candidate, source);
  let uploaded = false;
  if (!registry.version) {
    if (!registry.packageExists && !context.firstPublishOIDCReady) {
      if (!candidate.receipt.bootstrap_allowed)
        throw new Error("absent package is not an approved initial bootstrap");
      return { status: "bootstrap_required", candidate, source };
    }
    let writeError;
    try {
      await io.publish(candidate);
    } catch (error) {
      writeError = error;
    }
    registry = await observe(
      io,
      () => io.readRegistry(source.record.sdk_version),
      (value) => {
        verifyRegistry(value, candidate, source);
        return value.version !== null;
      },
      "npm publication",
      writeError,
    );
    uploaded = true;
  }
  if (
    !registry.version.provenance &&
    (uploaded || !candidate.receipt.bootstrap_allowed)
  )
    throw new Error("npm upload has no required workflow provenance");
  const origin = registry.version.provenance ? "oidc" : "manual_bootstrap";
  await ensureLatest(io, context);
  await io.verifyConsumers(candidate, origin);
  return {
    status: "verified",
    origin,
    sdk_version: source.record.sdk_version,
    host_release: source.record.host_release,
  };
}
