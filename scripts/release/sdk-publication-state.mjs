import { compareVersions, validateRecord } from "./plugin-sdk.mjs";
import { packageName } from "./sdk-registry.mjs";

export const REGISTRY_OBSERVATION_WINDOW_MS = 300_000;
export const REGISTRY_VALUE_LIMIT = 128;
const REGISTRY_PROBE_WAITS_MS = [10_000, 20_000, 30_000];

export function boundedRegistryValue(value) {
  if (typeof value !== "string") return null;
  return value.length > REGISTRY_VALUE_LIMIT
    ? `${value.slice(0, REGISTRY_VALUE_LIMIT - 3)}...`
    : value;
}

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

function probeWait(attempt) {
  return REGISTRY_PROBE_WAITS_MS[
    Math.min(attempt, REGISTRY_PROBE_WAITS_MS.length - 1)
  ];
}

function isDefiniteAuthenticationRejection(error) {
  const output = [error?.npmCode, error?.code, error?.stderr, error?.message]
    .filter((value) => typeof value === "string")
    .join("\n");
  return /\b(?:ENEEDAUTH|E401|E403)\b|unauthorized|authentication|permission denied|forbidden/i.test(
    output,
  );
}

function describeRegistryState(state) {
  return {
    package_exists: state.packageExists,
    observed_version: boundedRegistryValue(state.version?.version),
    observed_latest: boundedRegistryValue(state.latest),
  };
}

function reportObservation(io, event) {
  if (typeof io.diagnostics?.observation === "function")
    io.diagnostics.observation({
      package: packageName,
      ...(event.phase === "version_visibility"
        ? { write_outcome: io.diagnostics.writeOutcome }
        : {}),
      ...event,
    });
}

function visibilityTimeoutError(label, writeError, phase) {
  const minutes = REGISTRY_OBSERVATION_WINDOW_MS / 60_000;
  const error = writeError
    ? new Error(
        `${label} did not become visible within ${minutes} minutes after an uncertain npm response (${writeError.message}). Check the public registry identity before any recovery write.`,
      )
    : new Error(
        `${label} was accepted by npm but did not become publicly visible within ${minutes} minutes. Check https://www.npmjs.com/package/${packageName} and rerun the failed job in the same release.`,
      );
  error.code = "VISIBILITY_TIMEOUT";
  error.acceptedUpload = !writeError;
  error.phase = phase;
  error.operation = phase === "channel_promotion" ? "dist-tag" : "upload";
  error.writeError = writeError;
  return error;
}

async function observeRegistry(
  io,
  read,
  valid,
  { label, phase, version, describe = () => ({}) },
  writeError,
) {
  const started = io.now();
  const deadline = started + REGISTRY_OBSERVATION_WINDOW_MS;
  let attempt = 0;
  for (;;) {
    if (attempt > 0 && io.now() >= deadline) break;
    const value = await read();
    attempt += 1;
    if (valid(value)) {
      reportObservation(io, {
        phase,
        version,
        attempt,
        elapsed_ms: Math.round(io.now() - started),
        next_wait_ms: 0,
        visible: true,
        ...describe(value),
      });
      return value;
    }
    if (
      writeError &&
      attempt === 1 &&
      isDefiniteAuthenticationRejection(writeError)
    ) {
      writeError.phase = phase;
      writeError.operation =
        phase === "channel_promotion" ? "dist-tag" : "upload";
      throw writeError;
    }
    const now = io.now();
    const remaining = deadline - now;
    const nextWait =
      remaining > 0 ? Math.min(probeWait(attempt - 1), remaining) : 0;
    reportObservation(io, {
      phase,
      version,
      attempt,
      elapsed_ms: Math.round(now - started),
      next_wait_ms: nextWait,
      visible: false,
      ...describe(value),
    });
    if (nextWait <= 0) break;
    await io.registryWait(nextWait);
  }
  throw visibilityTimeoutError(label, writeError, phase);
}

export async function preparePublication(io, context) {
  validateContext(context);
  const { source } = context;
  io.diagnostics?.start("source_verification");
  let { tag, registry } = await readState(io, context);
  io.diagnostics?.phase("source_verification", "passed", {
    source_commit: source.commit,
    host_tag: source.hostTag,
  });
  io.diagnostics?.start("candidate_verification");
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
  await io.finalizeCandidate(candidate, source);
  io.diagnostics?.phase("candidate_verification", "passed", {
    integrity: candidate.integrity,
  });
  io.diagnostics?.setPublication("not_attempted");
  return { status: "prepared", candidate, source };
}

async function ensureLatest(io, context) {
  const version = context.source.record.sdk_version;
  io.diagnostics?.start("channel_promotion");
  try {
    let registry = await io.readRegistry(version);
    io.diagnostics?.setChannelWriteOutcome("not_attempted");
    if (registry.latest === version) {
      io.diagnostics?.phase("channel_promotion", "skipped", {
        reason: "already_current",
      });
      return;
    }
    if (registry.latest && compareVersions(registry.latest, version) > 0) {
      await io.verifyNewerLatest(registry.latest);
      io.diagnostics?.phase("channel_promotion", "skipped", {
        reason: "newer_channel_preserved",
      });
      return;
    }
    let writeError;
    try {
      await io.moveLatest(version);
      io.diagnostics?.setChannelWriteOutcome("accepted");
    } catch (error) {
      writeError = error;
      io.diagnostics?.setChannelWriteOutcome(
        isDefiniteAuthenticationRejection(error) ? "rejected" : "uncertain",
      );
    }
    registry = await observeRegistry(
      io,
      () => io.readRegistry(version),
      (state) =>
        state.latest === version ||
        (state.latest && compareVersions(state.latest, version) > 0),
      {
        label: "npm latest dist-tag",
        phase: "channel_promotion",
        version,
        describe: describeRegistryState,
      },
      writeError,
    );
    if (registry.latest !== version) await io.verifyNewerLatest(registry.latest);
    io.diagnostics?.phase("channel_promotion", "passed", {
      observed_latest: boundedRegistryValue(registry.latest),
    });
  } catch (error) {
    error.phase ??= "channel_promotion";
    error.operation ??= "dist-tag";
    throw error;
  }
}

export async function publishPublication(io, context) {
  validateContext(context);
  const { source } = context;
  io.diagnostics?.start("source_verification");
  let { tag, registry } = await readState(io, context);
  if (!tag)
    throw new Error("signed SDK tag is missing; run SDK preparation first");
  io.diagnostics?.phase("source_verification", "passed", {
    source_commit: source.commit,
    host_tag: source.hostTag,
  });
  io.diagnostics?.start("candidate_verification");
  const candidate = await io.obtainCandidate(source, {
    allowCreate: false,
    bootstrapAllowed: false,
  });
  verifyCandidate(candidate, context);
  verifyRegistry(registry, candidate, source);
  await io.verifyFinalizedCandidate(candidate, source);
  io.diagnostics?.phase("candidate_verification", "passed", {
    integrity: candidate.integrity,
  });
  let uploaded = false;
  if (!registry.version) {
    if (!registry.packageExists && !context.firstPublishOIDCReady) {
      if (!candidate.receipt.bootstrap_allowed)
        throw new Error("absent package is not an approved initial bootstrap");
      io.diagnostics?.phase("npm_upload", "skipped", {
        reason: "bootstrap_required",
      });
      io.diagnostics?.setPublication("not_attempted");
      return { status: "bootstrap_required", candidate, source };
    }
    io.diagnostics?.start("npm_upload");
    let writeError;
    try {
      await io.publish(candidate);
      io.diagnostics?.setWriteOutcome("accepted");
      io.diagnostics?.phase("npm_upload", "passed", {
        write_outcome: "accepted",
      });
    } catch (error) {
      writeError = error;
      const rejected = isDefiniteAuthenticationRejection(error);
      const outcome = rejected ? "rejected" : "uncertain";
      io.diagnostics?.setWriteOutcome(outcome);
      if (rejected)
        io.diagnostics?.phase("npm_upload", "failed", {
          write_outcome: outcome,
        });
    }
    io.diagnostics?.start("version_visibility");
    registry = await observeRegistry(
      io,
      () => io.readRegistry(source.record.sdk_version),
      (value) => {
        verifyRegistry(value, candidate, source);
        return value.version !== null;
      },
      {
        label: "npm publication",
        phase: "version_visibility",
        version: source.record.sdk_version,
        describe: describeRegistryState,
      },
      writeError,
    );
    io.diagnostics?.phase("npm_upload", "passed", {
      write_outcome: writeError ? "uncertain" : "accepted",
    });
    io.diagnostics?.phase("version_visibility", "passed", {
      observed_version: boundedRegistryValue(registry.version?.version),
      observed_latest: boundedRegistryValue(registry.latest),
    });
    io.diagnostics?.setPublication("confirmed");
    uploaded = true;
  } else {
    io.diagnostics?.phase("npm_upload", "skipped", {
      reason: "version_already_public",
    });
    io.diagnostics?.phase("version_visibility", "passed", {
      observed_version: boundedRegistryValue(registry.version?.version),
      observed_latest: boundedRegistryValue(registry.latest),
    });
    io.diagnostics?.setPublication("confirmed");
  }
  if (
    !registry.version.provenance &&
    (uploaded || !candidate.receipt.bootstrap_allowed)
  )
    throw new Error("npm upload has no required workflow provenance");
  const origin = registry.version.provenance ? "oidc" : "manual_bootstrap";
  await ensureLatest(io, context);
  io.diagnostics?.start("public_consumers");
  await io.verifyConsumers(candidate, origin);
  io.diagnostics?.phase("public_consumers", "passed", { origin });
  return {
    status: "verified",
    origin,
    sdk_version: source.record.sdk_version,
    host_release: source.record.host_release,
  };
}
