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
  AgentProfileKind,
  AgentProfilePayload,
  CLIFlag,
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
      return [
        {
          position,
          executionProfileId: agentProfileId(executionProfileId),
          enabled: typeof item.enabled === "boolean" ? item.enabled : true,
          rules:
            rules && typeof rules === "object" && !Array.isArray(rules)
              ? Object.fromEntries(
                  Object.entries(rules).filter(
                    (entry): entry is [string, string] =>
                      typeof entry[0] === "string" && typeof entry[1] === "string",
                  ),
                )
              : {},
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
      candidates: profile.dynamic.candidates.map((candidate) => ({
        position: candidate.position,
        execution_profile_id: candidate.executionProfileId,
        enabled: candidate.enabled,
        rules: candidate.rules,
      })),
    };
  }
  return payload;
}
