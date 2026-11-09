import { Buffer } from "node:buffer";

export const OUTPUT_LIMIT_BYTES = 64 * 1024;
export const PHASES = [
  "source_verification",
  "candidate_verification",
  "npm_upload",
  "version_visibility",
  "channel_promotion",
  "public_consumers",
];

export const FAILURE_GUIDANCE = {
  authentication:
    "Inspect this package's trusted publisher configuration on npmjs.com: repository, workflow, environment, expiration, and allowed actions, including dist-tag changes when promoting a channel.",
  accepted_upload_awaiting_visibility:
    "Check the public package version on npmjs.com. Then rerun the failed job in the same release; do not publish again.",
  uncertain_upload:
    "Check the public registry identity for the exact version before any recovery write.",
  accepted_channel_awaiting_visibility:
    "Check the public latest dist-tag. Then rerun the failed job in the same release; do not repeat the dist-tag update.",
  uncertain_channel_update:
    "Check the public latest dist-tag before any recovery dist-tag write.",
  registry_read:
    "Restore readable public registry access, then retry the same release.",
  identity_conflict:
    "Stop recovery and investigate the existing immutable tag, receipt, and archive evidence.",
  consumer_verification:
    "Inspect the named public consumer check and rerun verification after correction.",
  bootstrap_required:
    "Verify the retained receipt and archive, complete the documented first-package bootstrap, configure trusted publishing, then rerun the same release. No SDK completion has been recorded.",
  unknown: "Inspect the failed phase and rerun the same release.",
};

const SECRET_KEY = /TOKEN|SECRET|PASSWORD|PASSPHRASE|PRIVATE_KEY|AUTH/i;

export function collectSecrets(env = process.env) {
  return Object.entries(env)
    .filter(
      ([key, value]) =>
        SECRET_KEY.test(key) &&
        typeof value === "string" &&
        value.length >= 8,
    )
    .map(([, value]) => value)
    .sort((left, right) => right.length - left.length);
}

function redactURL(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    return "[redacted-url]";
  }
  const sensitive = [...url.searchParams.keys()].some((key) =>
    /token|secret|signature|sig|auth|password|key/i.test(key),
  );
  if (url.username || url.password || sensitive) return "[redacted-url]";
  return raw;
}

function sanitizeText(value, secrets) {
  let text = value;
  for (const secret of secrets) text = text.split(secret).join("[redacted]");
  return text
    .replace(
      /\b((?:proxy-)?authorization)["']?\s*:\s*(?:"[^"\r\n]*"|'[^'\r\n]*'|(?:(?:bearer|basic)\s+)?[^\r\n"',;]+)/gi,
      (_match, header) => `${header}: [redacted]`,
    )
    .replace(/\bbearer\s+[A-Za-z0-9._~+/=-]{8,}/gi, "Bearer [redacted]")
    .replace(/\bBasic\s+[A-Za-z0-9+/=]{8,}/g, "Basic [redacted]")
    .replace(
      /^(\s*(?:@[^:\s]+:)?(?:\/\/[^\s=]+:)?(?:_authToken|_auth|_password|password))\s*=\s*.+$/gim,
      "$1=[redacted]",
    )
    .replace(
      /(["']?(?:access_token|id_token|refresh_token|token|password|secret)["']?\s*[:=]\s*["']?)[^"'\s,}]+/gi,
      "$1[redacted]",
    )
    .replace(
      /\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b/g,
      "[redacted-jwt]",
    )
    .replace(/\bnpm_[A-Za-z0-9]{20,}\b/g, "[redacted-npm-token]")
    .replace(
      /\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b/g,
      "[redacted-github-token]",
    )
    .replace(/\bhttps?:\/\/[^\s"'<>]+/gi, redactURL);
}

export function sanitizeOutput(
  value,
  { secrets = [], limit = OUTPUT_LIMIT_BYTES } = {},
) {
  const text = sanitizeText(typeof value === "string" ? value : "", secrets);
  const bytes = Buffer.byteLength(text, "utf8");
  if (bytes <= limit)
    return {
      text,
      truncated: false,
      original_bytes: bytes,
      retained_bytes: bytes,
    };
  const marker = `[truncated: showing the last ${limit} bytes]\n`;
  const markerBytes = Buffer.byteLength(marker, "utf8");
  const tailBytes = Math.max(0, limit - markerBytes);
  const encoded = Buffer.from(text, "utf8");
  let start = Math.max(0, bytes - tailBytes);
  while (start < encoded.length && (encoded[start] & 0xc0) === 0x80) start += 1;
  const tail = encoded.subarray(start).toString("utf8");
  return {
    text: marker + tail,
    truncated: true,
    original_bytes: bytes,
    retained_bytes: Buffer.byteLength(marker + tail, "utf8"),
  };
}

const AUTH_CODE = /^(?:ENEEDAUTH|E401|E403)$/;
const AUTH_TEXT =
  /\b(?:ENEEDAUTH|E401|E403)\b|unauthorized|forbidden|authentication|permission denied|trusted publisher|publisher/i;
const REGISTRY_TEXT =
  /registry (?:read|returned|metadata|URL)|registry archive download/i;
const CONSUMER_TEXT = /public (?:Go|frontend|consumer)|consumer verification/i;
const IDENTITY_TEXT =
  /conflict|signature|integrity|no required workflow provenance|provenance is absent|invalid provenance|provenance (?:verification|validation|bundle|attestation)/i;

function failureResult(category, error, secrets = []) {
  const message = sanitizeOutput(
    error?.message ?? "unknown SDK publication failure",
    { secrets },
  );
  return {
    category,
    message: message.text,
    recovery_action: FAILURE_GUIDANCE[category],
  };
}

export function classifyFailure(error, { secrets = [] } = {}) {
  const explicit =
    error?.category === "npm_authentication"
      ? "authentication"
      : error?.category;
  if (explicit && FAILURE_GUIDANCE[explicit])
    return failureResult(explicit, error, secrets);
  const text = [
    error?.npmCode,
    error?.code,
    error?.message,
    error?.stderr,
    error?.stdout,
  ]
    .filter((value) => typeof value === "string" || typeof value === "number")
    .join("\n");
  if (error?.code === "VISIBILITY_TIMEOUT") {
    const channel =
      error.phase === "channel_promotion" || error.operation === "dist-tag";
    const category = channel
      ? error.acceptedUpload
        ? "accepted_channel_awaiting_visibility"
        : "uncertain_channel_update"
      : error.acceptedUpload
        ? "accepted_upload_awaiting_visibility"
        : "uncertain_upload";
    return failureResult(category, error, secrets);
  }
  if (error?.code === "REGISTRY_READ_FAILURE" || REGISTRY_TEXT.test(text))
    return failureResult("registry_read", error, secrets);
  if (error?.code === "CONSUMER_VERIFICATION" || CONSUMER_TEXT.test(text))
    return failureResult("consumer_verification", error, secrets);
  if (AUTH_CODE.test(error?.npmCode ?? "") || AUTH_TEXT.test(text))
    return failureResult("authentication", error, secrets);
  if (IDENTITY_TEXT.test(text))
    return failureResult("identity_conflict", error, secrets);
  return failureResult("unknown", error, secrets);
}

export class SdkDiagnostics {
  constructor({
    hostTag,
    sdkVersion = null,
    packageName: packageName = "@kdlbs/plugin-sdk",
    secrets = [],
    log = console.log,
  } = {}) {
    this.hostTag = hostTag;
    this.sdkVersion = sdkVersion;
    this.packageName = packageName;
    this.secrets = secrets;
    this.log = log;
    this.phases = Object.fromEntries(PHASES.map((phase) => [phase, "pending"]));
    this.current = null;
    this.writeOutcome = "not_attempted";
    this.channelWriteOutcome = "not_attempted";
    this.publication = "unknown";
    this.failure = null;
    this.observations = [];
    this.commands = [];
  }
  setSdkVersion(version) {
    this.sdkVersion = version;
  }
  start(phase) {
    this.assertPhase(phase);
    this.current = phase;
    this.phases[phase] = "pending";
  }
  phase(phase, status, details = {}) {
    this.assertPhase(phase);
    this.phases[phase] = status;
    if (this.current === phase && status !== "pending") this.current = null;
    this.log?.(JSON.stringify({ event: "sdk_release_phase", phase, status, ...details }));
  }
  observation(event) {
    const record = { event: "sdk_registry_observation", ...event };
    this.observations.push(record);
    if (this.observations.length > 40) this.observations.shift();
    this.log?.(JSON.stringify(record));
  }
  command({ args, outcome, exitCode = 0, output }) {
    const record = {
      event: "sdk_release_command",
      args,
      outcome,
      exit_code: exitCode,
      output: output?.text ?? "",
      truncated: output?.truncated ?? false,
    };
    this.commands.push(record);
    this.log?.(JSON.stringify(record));
  }
  setWriteOutcome(outcome) {
    this.writeOutcome = outcome;
  }
  setChannelWriteOutcome(outcome) {
    this.channelWriteOutcome = outcome;
  }
  setPublication(state) {
    this.publication = state;
  }
  fail(error) {
    const classification = classifyFailure(error, { secrets: this.secrets });
    this.failure = classification;
    if (this.current) {
      this.phases[this.current] = "failed";
      this.current = null;
    }
    const channelFailure =
      error?.phase === "channel_promotion" || error?.operation === "dist-tag";
    if (channelFailure) {
      if (error?.code === "VISIBILITY_TIMEOUT")
        this.channelWriteOutcome = error.acceptedUpload
          ? "accepted"
          : "uncertain";
      else if (classification.category === "authentication")
        this.channelWriteOutcome = "rejected";
      return classification;
    }
    if (error?.code === "VISIBILITY_TIMEOUT") {
      this.publication = error.acceptedUpload
        ? "accepted_upload_awaiting_visibility"
        : "uncertain_upload";
    } else if (classification.category === "authentication") {
      this.publication = "rejected";
    } else if (
      classification.category === "registry_read" &&
      this.publication === "unknown"
    ) {
      this.publication =
        this.writeOutcome === "accepted"
          ? "accepted_upload_unverified"
          : this.writeOutcome === "uncertain"
            ? "uncertain_upload"
            : "unknown";
    }
    return classification;
  }
  summary({ status = "failed", result = null, failure = null } = {}) {
    const classification = failure ? this.failure ?? this.fail(failure) : null;
    const lastConfirmedPhase =
      [...PHASES].reverse().find((phase) => this.phases[phase] === "passed") ??
      null;
    const summary = {
      status,
      package: this.packageName,
      host_tag: this.hostTag,
      sdk_version: this.sdkVersion,
      phases: { ...this.phases },
      publication: this.publication,
      write_outcome: this.writeOutcome,
      channel_write_outcome: this.channelWriteOutcome,
      channel_promotion: this.phases.channel_promotion,
      public_consumers: this.phases.public_consumers,
      last_confirmed_phase: lastConfirmedPhase,
      recovery_action:
        classification?.recovery_action ??
        (status === "bootstrap_required"
          ? FAILURE_GUIDANCE.bootstrap_required
          : failure || status === "failed"
            ? FAILURE_GUIDANCE.unknown
            : "No action required."),
    };
    if (classification) {
      summary.failure_category = classification.category;
      summary.failure_message = classification.message;
    }
    if (this.observations.length)
      summary.observations = this.observations.slice(-20);
    if (this.commands.length) summary.commands = this.commands;
    if (result?.sdk_version) summary.sdk_version = result.sdk_version;
    if (result?.host_release) summary.host_release = result.host_release;
    if (result?.origin) summary.origin = result.origin;
    if (result?.status) summary.release_status = result.status;
    return summary;
  }
  assertPhase(phase) {
    if (!(phase in this.phases))
      throw new Error(`unknown SDK release phase: ${phase}`);
  }
}
