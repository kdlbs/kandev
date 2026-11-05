/**
 * Versioned wire contract between the Kandev-supplied OpenCode plugin and the
 * agentctl launch host. The plugin never accepts arbitrary module methods:
 * every payload is a closed union for a compiled behavior version.
 */

export const PROTOCOL_VERSION = 1;

export const SHELL_OUTCOMES_BEHAVIOR = "shell-outcomes";
export const SHELL_OUTCOMES_VERSION = 1;

export const MAX_EVENT_REQUEST_BYTES = 64 * 1024;
export const MAX_EVENTS_PER_REQUEST = 64;
export const MAX_OUTBOX_OBSERVATIONS = 256;
export const MAX_OUTBOX_BYTES = 1024 * 1024;
export const MAX_UNCLAIMED_FACTS = 256;
export const MAX_UNCLAIMED_BYTES = 1024 * 1024;
export const UNCLAIMED_FACT_LIFETIME_MS = 60_000;
export const CONFIG_POLL_TIMEOUT_MS = 20_000;
export const REQUEST_TIMEOUT_MS = 10_000;
export const RETRY_INITIAL_MS = 250;
export const RETRY_MAX_MS = 5_000;

export class ProtocolError extends Error {
  readonly code: string;

  constructor(code: string, message: string) {
    super(message);
    this.name = "ProtocolError";
    this.code = code;
  }
}

export interface BehaviorRef {
  readonly id: string;
  readonly version: number;
}

export interface Bootstrap {
  readonly protocolVersion: number;
  readonly endpoint: string;
  readonly credential: string;
  readonly leaseId: string;
  readonly artifactHash: string;
  readonly behaviors: readonly BehaviorRef[];
}

export interface HelloRequest {
  readonly protocolVersion: number;
  readonly leaseId: string;
  readonly artifactHash: string;
  readonly runtime: {
    readonly family: string;
    readonly version: string;
  };
  readonly behaviors: readonly BehaviorRef[];
}

export interface HelloResponse {
  readonly protocolVersion: number;
  readonly leaseId: string;
  readonly ready: boolean;
  readonly behaviors: readonly BehaviorRef[];
}

export interface ShellClaim {
  readonly shellId: string;
  readonly nativeSessionId: string;
  readonly nativeGeneration: number;
  readonly workId: string;
  readonly runId: string;
  readonly sourceCallId: string;
  readonly title: string;
  readonly promptGeneration: number;
  readonly submissionId: string;
}

export interface BehaviorConfig {
  readonly version: number;
  /** Behavior-owned configuration; the shared core never interprets it. */
  readonly data?: unknown;
}

export interface ConfigResponse {
  readonly protocolVersion: number;
  readonly leaseId: string;
  readonly revision: number;
  readonly behaviors: Readonly<Record<string, BehaviorConfig>>;
}

export type ShellTerminalStatus = "exited" | "timeout" | "killed";

export interface ShellTerminalPayload {
  readonly kind: "shell.terminal";
  readonly shellId: string;
  readonly status: ShellTerminalStatus;
  readonly exitCode?: number;
  readonly startedAt?: number;
  readonly completedAt?: number;
}

export type ObservationPayload = unknown;

export interface Observation {
  readonly eventId: string;
  readonly behaviorId: string;
  readonly behaviorVersion: number;
  readonly sequence: number;
  readonly nativeSessionId: string;
  readonly nativeGeneration: number;
  readonly payload: ObservationPayload;
}

export interface EventRequest {
  readonly protocolVersion: number;
  readonly leaseId: string;
  readonly producerId: string;
  readonly events: readonly Observation[];
}

export interface EventRejection {
  readonly eventId: string;
  readonly code: string;
}

export interface EventResponse {
  readonly accepted: readonly string[];
  readonly rejected: readonly EventRejection[];
}

const LOOPBACK_HOSTS = new Set(["127.0.0.1", "::1", "[::1]"]);

export function decodeBootstrap(value: unknown): Bootstrap {
  const record = asRecord(value, "bootstrap");
  const protocolVersion = asInteger(record.protocolVersion, "bootstrap.protocolVersion");
  if (protocolVersion !== PROTOCOL_VERSION) {
    throw new ProtocolError("protocol_mismatch", `unsupported protocol version ${protocolVersion}`);
  }
  const endpoint = asString(record.endpoint, "bootstrap.endpoint");
  const parsed = new URL(endpoint);
  if (parsed.protocol !== "http:" || !LOOPBACK_HOSTS.has(parsed.hostname)) {
    throw new ProtocolError(
      "endpoint_not_loopback",
      "bootstrap endpoint must be a loopback HTTP URL",
    );
  }
  if (!parsed.port) {
    throw new ProtocolError("endpoint_port_missing", "bootstrap endpoint must name a port");
  }
  const credential = asString(record.credential, "bootstrap.credential");
  if (credential.length < 16) {
    throw new ProtocolError("credential_invalid", "bootstrap credential is missing or too short");
  }
  const leaseId = asString(record.leaseId, "bootstrap.leaseId");
  const artifactHash = asString(record.artifactHash, "bootstrap.artifactHash");
  const behaviors = decodeBehaviorRefs(record.behaviors, "bootstrap.behaviors");
  return {
    protocolVersion,
    endpoint: parsed.toString().replace(/\/$/, ""),
    credential,
    leaseId,
    artifactHash,
    behaviors,
  };
}

export function decodeHelloResponse(value: unknown, leaseId: string): HelloResponse {
  const record = asRecord(value, "hello response");
  const protocolVersion = asInteger(record.protocolVersion, "hello.protocolVersion");
  if (protocolVersion !== PROTOCOL_VERSION) {
    throw new ProtocolError("protocol_mismatch", "host protocol version does not match");
  }
  if (asString(record.leaseId, "hello.leaseId") !== leaseId) {
    throw new ProtocolError("lease_mismatch", "hello response names a different launch lease");
  }
  const behaviors = decodeBehaviorRefs(record.behaviors, "hello.behaviors");
  return {
    protocolVersion,
    leaseId,
    ready: record.ready === true,
    behaviors,
  };
}

export function decodeConfigResponse(value: unknown, leaseId: string): ConfigResponse {
  const record = asRecord(value, "config response");
  const protocolVersion = asInteger(record.protocolVersion, "config.protocolVersion");
  if (protocolVersion !== PROTOCOL_VERSION) {
    throw new ProtocolError("protocol_mismatch", "host protocol version does not match");
  }
  if (asString(record.leaseId, "config.leaseId") !== leaseId) {
    throw new ProtocolError("lease_mismatch", "config response names a different launch lease");
  }
  const revision = asInteger(record.revision, "config.revision");
  const behaviorValues = asRecord(record.behaviors, "config.behaviors");
  const behaviors: Record<string, BehaviorConfig> = {};
  for (const [id, raw] of Object.entries(behaviorValues)) {
    const behavior = asRecord(raw, `config.behaviors.${id}`);
    const version = asInteger(behavior.version, `config.behaviors.${id}.version`);
    behaviors[id] = { version, data: behavior.data };
  }
  return { protocolVersion, leaseId, revision, behaviors };
}

export function decodeEventResponse(value: unknown): EventResponse {
  const record = asRecord(value, "event response");
  const accepted = asStringArray(record.accepted, "events.accepted");
  const rejectedRaw = record.rejected ?? [];
  if (!Array.isArray(rejectedRaw)) {
    throw new ProtocolError("events_invalid", "events.rejected must be an array");
  }
  const rejected = rejectedRaw.map((item, index) => {
    const rejection = asRecord(item, `events.rejected[${index}]`);
    return {
      eventId: asString(rejection.eventId, `events.rejected[${index}].eventId`),
      code: asString(rejection.code, `events.rejected[${index}].code`),
    };
  });
  return { accepted, rejected };
}

export function encodeObservationBytes(observation: Observation): number {
  return Buffer.byteLength(JSON.stringify(observation), "utf8");
}

export function decodeShellClaims(data: unknown): ShellClaim[] {
  const record = asRecord(data ?? {}, "shell config");
  const values = record.claims ?? [];
  if (!Array.isArray(values)) {
    throw new ProtocolError("config_invalid", "shell config claims must be an array");
  }
  return values
    .slice(0, 256)
    .map((claim, index) => decodeClaim(claim, `shell config claims[${index}]`));
}

function decodeClaim(value: unknown, path: string): ShellClaim {
  const record = asRecord(value, path);
  const claim: ShellClaim = {
    shellId: asString(record.shellId, `${path}.shellId`),
    nativeSessionId: asString(record.nativeSessionId, `${path}.nativeSessionId`),
    nativeGeneration: asInteger(record.nativeGeneration, `${path}.nativeGeneration`),
    workId: asOptionalString(record.workId),
    runId: asOptionalString(record.runId),
    sourceCallId: asOptionalString(record.sourceCallId),
    title: asOptionalString(record.title),
    promptGeneration: asOptionalInteger(record.promptGeneration),
    submissionId: asOptionalString(record.submissionId),
  };
  if (claim.nativeGeneration <= 0) {
    throw new ProtocolError("config_invalid", `${path}.nativeGeneration must be positive`);
  }
  return claim;
}

function decodeBehaviorRefs(value: unknown, path: string): BehaviorRef[] {
  if (!Array.isArray(value)) {
    throw new ProtocolError("protocol_invalid", `${path} must be an array`);
  }
  return value.map((item, index) => {
    const record = asRecord(item, `${path}[${index}]`);
    return {
      id: asString(record.id, `${path}[${index}].id`),
      version: asInteger(record.version, `${path}[${index}].version`),
    };
  });
}

function asRecord(value: unknown, path: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new ProtocolError("protocol_invalid", `${path} must be an object`);
  }
  return value as Record<string, unknown>;
}

function asString(value: unknown, path: string): string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new ProtocolError("protocol_invalid", `${path} must be a non-empty string`);
  }
  return value;
}

function asInteger(value: unknown, path: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new ProtocolError("protocol_invalid", `${path} must be a non-negative integer`);
  }
  return value;
}

function asOptionalString(value: unknown): string {
  return typeof value === "string" ? value : "";
}

function asOptionalInteger(value: unknown): number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : 0;
}

function asStringArray(value: unknown, path: string): string[] {
  if (!Array.isArray(value)) {
    throw new ProtocolError("protocol_invalid", `${path} must be an array`);
  }
  return value.map((item, index) => asString(item, `${path}[${index}]`));
}
