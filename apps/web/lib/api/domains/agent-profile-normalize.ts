// Kanban-side AgentProfile normalizer (ADR 0005, Wave E):
// converts the snake_case payloads served by `/api/v1/agents` and
// `/api/v1/agent-profiles/:id` into the canonical camelCase
// `AgentProfile` shape.
//
// Wired via thin wrappers around the server actions / WS payloads in
// `app/actions/agents.ts` and `lib/ws/handlers/agents.ts`.

import type { ProfileEnvVar } from "@/lib/types/http";
import type {
  AgentProfile,
  DynamicAgentPolicy,
  AgentProfileKind,
  AgentProfilePayload,
  CLIFlag,
  DynamicErrorPolicy,
  DynamicPolicyOutcome,
  DynamicAgentProfile,
} from "@/lib/types/agent-profile";
import { agentProfileId, workspaceId as toWorkspaceId } from "@/lib/types/ids";

type RawProfile = Partial<AgentProfilePayload> & Partial<AgentProfile> & Record<string, unknown>;

function pickString(raw: RawProfile, camel: string, snake: string, fallback = ""): string {
  const value = raw[camel] ?? raw[snake];
  return typeof value === "string" ? value : fallback;
}

function pickBool(raw: RawProfile, camel: string, snake: string, fallback = false): boolean {
  const value = raw[camel] ?? raw[snake];
  return typeof value === "boolean" ? value : fallback;
}

function pickOptionalString(raw: RawProfile, camel: string, snake: string): string | undefined {
  const value = raw[camel] ?? raw[snake];
  return typeof value === "string" ? value : undefined;
}

function pickFlags(raw: RawProfile): CLIFlag[] {
  const value = raw.cliFlags ?? raw.cli_flags;
  return Array.isArray(value) ? (value as CLIFlag[]) : [];
}

function pickEnvVars(raw: RawProfile): ProfileEnvVar[] {
  const value = raw.envVars ?? raw.env_vars;
  return Array.isArray(value) ? (value as ProfileEnvVar[]) : [];
}

function pickConfigOptions(raw: RawProfile): Record<string, string> | undefined {
  const value = raw.configOptions ?? raw.config_options;
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const entries = Object.entries(value).filter(
    (entry): entry is [string, string] =>
      typeof entry[0] === "string" && typeof entry[1] === "string" && entry[0] !== "",
  );
  return entries.length > 0 ? Object.fromEntries(entries) : undefined;
}

const TRANSIENT_CODES = new Set([
  "network_unavailable",
  "provider_unavailable",
  "provider_overloaded",
  "model_capacity",
  "rate_limited",
  "agent_transport_lost",
]);

const HARD_CODES = new Set([
  "auth_required",
  "missing_credentials",
  "subscription_required",
  "quota_limited",
  "model_unavailable",
  "provider_not_configured",
]);

function defaultDynamicErrorPolicy(): DynamicErrorPolicy {
  return {
    retry: { enabled: false, maxRetries: 0, initialIntervalSeconds: 0 },
    waitForReset: { enabled: false, maxWaitSeconds: 0 },
    onExhausted: "skip",
  };
}

function policyForLegacyAction(action: string): DynamicErrorPolicy {
  const policy = defaultDynamicErrorPolicy();
  if (action === "retry_same") {
    policy.retry = { enabled: true, maxRetries: 1, initialIntervalSeconds: 5 };
    policy.onExhausted = "stop";
  } else if (action === "stop") {
    policy.onExhausted = "stop";
  }
  return policy;
}

function legacyRulesToPolicy(rules: Record<string, string>): DynamicAgentPolicy {
  const generic = rules.on_provider_error;
  const result: DynamicAgentPolicy = {
    version: 1,
    transient: policyForLegacyAction(generic ?? "try_next"),
    hard: policyForLegacyAction(generic ?? "try_next"),
  };
  for (const [code, action] of Object.entries(rules)) {
    if (code === "on_provider_error") continue;
    if (TRANSIENT_CODES.has(code)) result.transient = policyForLegacyAction(action);
    if (HARD_CODES.has(code)) result.hard = policyForLegacyAction(action);
  }
  return result;
}

function objectValue(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function numberValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function normalizeDynamicRetryPolicy(source: Record<string, unknown>): DynamicErrorPolicy["retry"] {
  const retry = objectValue(source.retry) ?? {};
  const retrySnake = objectValue(source.retry_policy) ?? {};
  return {
    enabled: (retry.enabled ?? retrySnake.enabled) === true,
    maxRetries: numberValue(
      retry.maxRetries ?? retry.max_retries ?? retrySnake.maxRetries ?? retrySnake.max_retries,
      0,
    ),
    initialIntervalSeconds: numberValue(
      retry.initialIntervalSeconds ??
        retry.initial_interval_seconds ??
        retrySnake.initialIntervalSeconds ??
        retrySnake.initial_interval_seconds,
      0,
    ),
  };
}

function normalizeDynamicWaitPolicy(
  source: Record<string, unknown>,
): DynamicErrorPolicy["waitForReset"] {
  const wait = objectValue(source.waitForReset) ?? objectValue(source.wait_for_reset) ?? {};
  return {
    enabled: wait.enabled === true,
    maxWaitSeconds: numberValue(wait.maxWaitSeconds ?? wait.max_wait_seconds, 0),
  };
}

function normalizeDynamicErrorPolicy(raw: unknown): DynamicErrorPolicy {
  const source = objectValue(raw) ?? {};
  const outcome = source.onExhausted ?? source.on_exhausted;
  return {
    retry: normalizeDynamicRetryPolicy(source),
    waitForReset: normalizeDynamicWaitPolicy(source),
    onExhausted: outcome === "stop" ? "stop" : "skip",
  };
}

function normalizeDynamicPolicy(
  raw: unknown,
  legacyRules: Record<string, string>,
): DynamicAgentPolicy {
  const source = objectValue(raw);
  if (!source) return legacyRulesToPolicy(legacyRules);
  return {
    version: numberValue(source.version, 1),
    transient: normalizeDynamicErrorPolicy(source.transient),
    hard: normalizeDynamicErrorPolicy(source.hard),
  };
}

function pickDynamic(raw: RawProfile): DynamicAgentProfile | undefined {
  const value = raw.dynamic;
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const document = value as Record<string, unknown>;
  const version = typeof document.version === "number" ? document.version : 0;
  const candidates = Array.isArray(document.candidates) ? document.candidates : [];
  return {
    version,
    candidates: candidates.flatMap((candidate) => {
      if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) return [];
      const item = candidate as Record<string, unknown>;
      const executionProfileId = item.executionProfileId ?? item.execution_profile_id;
      const position = item.position;
      if (typeof executionProfileId !== "string" || typeof position !== "number") return [];
      const rules = item.rules;
      const legacyRules =
        rules && typeof rules === "object" && !Array.isArray(rules)
          ? Object.fromEntries(
              Object.entries(rules).filter(
                (entry): entry is [string, string] =>
                  typeof entry[0] === "string" && typeof entry[1] === "string",
              ),
            )
          : {};
      return [
        {
          position,
          executionProfileId: agentProfileId(executionProfileId),
          enabled: typeof item.enabled === "boolean" ? item.enabled : true,
          policies: normalizeDynamicPolicy(item.policies ?? item.policy, legacyRules),
        },
      ];
    }),
  };
}

/**
 * Convert a kanban snake_case payload (or a partially-camelCased one) into
 * the canonical `AgentProfile`. Office-orchestration fields are left
 * undefined — kanban rows do not carry them.
 */
export function normalizeAgentProfile(raw: unknown): AgentProfile {
  const profile = (raw ?? {}) as RawProfile;
  const kind = pickString(profile, "kind", "kind");
  const dynamic = pickDynamic(profile);
  return {
    id: agentProfileId(pickString(profile, "id", "id")),
    ...(kind === "dynamic" || kind === "concrete" ? { kind: kind as AgentProfileKind } : {}),
    name: pickString(profile, "name", "name"),
    agentId: pickString(profile, "agentId", "agent_id"),
    agentDisplayName: pickString(profile, "agentDisplayName", "agent_display_name"),
    model: pickString(profile, "model", "model"),
    fallbackModel: pickString(profile, "fallbackModel", "fallback_model"),
    autoFallback: pickBool(profile, "autoFallback", "auto_fallback"),
    mode: (profile.mode as string | undefined) ?? undefined,
    configOptions: pickConfigOptions(profile),
    allowIndexing: pickBool(profile, "allowIndexing", "allow_indexing"),
    autoApprove: pickBool(profile, "autoApprove", "auto_approve"),
    cliFlags: pickFlags(profile),
    commandPrefix: pickOptionalString(profile, "commandPrefix", "command_prefix"),
    envVars: pickEnvVars(profile),
    cliPassthrough: pickBool(profile, "cliPassthrough", "cli_passthrough"),
    // Absent on legacy payloads → enabled by default.
    enabled: pickBool(profile, "enabled", "enabled", true),
    workspaceId: (() => {
      const value = pickOptionalString(profile, "workspaceId", "workspace_id");
      return value ? toWorkspaceId(value) : undefined;
    })(),
    userModified: (profile.userModified ?? profile.user_modified) as boolean | undefined,
    createdAt: pickString(profile, "createdAt", "created_at"),
    updatedAt: pickString(profile, "updatedAt", "updated_at"),
    ...(dynamic ? { dynamic } : {}),
  };
}

function setPayloadField<K extends keyof AgentProfilePayload>(
  payload: Partial<AgentProfilePayload>,
  key: K,
  value: AgentProfilePayload[K] | undefined,
) {
  if (value !== undefined) {
    payload[key] = value;
  }
}

/**
 * Inverse of `normalizeAgentProfile` — convert the canonical shape back to
 * a snake_case wire payload for `POST/PATCH` to the kanban endpoints.
 */
export function toAgentProfilePayload(
  profile: Partial<AgentProfile>,
): Partial<AgentProfilePayload> {
  const payload: Partial<AgentProfilePayload> = {};
  setPayloadField(payload, "id", profile.id);
  setPayloadField(payload, "kind", profile.kind);
  setPayloadField(payload, "name", profile.name);
  setPayloadField(payload, "agent_id", profile.agentId);
  setPayloadField(payload, "agent_display_name", profile.agentDisplayName);
  setPayloadField(payload, "model", profile.model);
  setPayloadField(payload, "fallback_model", profile.fallbackModel);
  setPayloadField(payload, "auto_fallback", profile.autoFallback);
  setPayloadField(payload, "mode", profile.mode);
  setPayloadField(payload, "config_options", profile.configOptions);
  setPayloadField(payload, "allow_indexing", profile.allowIndexing);
  setPayloadField(payload, "auto_approve", profile.autoApprove);
  setPayloadField(payload, "cli_flags", profile.cliFlags);
  setPayloadField(payload, "command_prefix", profile.commandPrefix);
  setPayloadField(payload, "env_vars", profile.envVars);
  setPayloadField(payload, "cli_passthrough", profile.cliPassthrough);
  setPayloadField(payload, "enabled", profile.enabled);
  setPayloadField(payload, "user_modified", profile.userModified);
  setPayloadField(payload, "created_at", profile.createdAt);
  setPayloadField(payload, "updated_at", profile.updatedAt);
  if (profile.dynamic) {
    payload.dynamic = {
      version: profile.dynamic.version,
      candidates: profile.dynamic.candidates.map((candidate) => {
        const policy = candidate.policies ?? legacyRulesToPolicy(candidate.rules ?? {});
        return {
          position: candidate.position,
          execution_profile_id: candidate.executionProfileId,
          enabled: candidate.enabled,
          policies: {
            version: policy.version,
            transient: {
              retry: {
                enabled: policy.transient.retry.enabled,
                max_retries: policy.transient.retry.maxRetries,
                initial_interval_seconds: policy.transient.retry.initialIntervalSeconds,
              },
              wait_for_reset: {
                enabled: policy.transient.waitForReset.enabled,
                max_wait_seconds: policy.transient.waitForReset.maxWaitSeconds,
              },
              on_exhausted: policy.transient.onExhausted as DynamicPolicyOutcome,
            },
            hard: {
              retry: {
                enabled: policy.hard.retry.enabled,
                max_retries: policy.hard.retry.maxRetries,
                initial_interval_seconds: policy.hard.retry.initialIntervalSeconds,
              },
              wait_for_reset: {
                enabled: policy.hard.waitForReset.enabled,
                max_wait_seconds: policy.hard.waitForReset.maxWaitSeconds,
              },
              on_exhausted: policy.hard.onExhausted as DynamicPolicyOutcome,
            },
          },
        };
      }),
    };
  }
  return payload;
}
