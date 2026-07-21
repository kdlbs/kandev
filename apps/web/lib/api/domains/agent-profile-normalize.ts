// Kanban-side AgentProfile normalizer (ADR 0005, Wave E):
// converts the snake_case payloads served by `/api/v1/agents` and
// `/api/v1/agent-profiles/:id` into the canonical camelCase
// `AgentProfile` shape.
//
// Wired via thin wrappers around the server actions / WS payloads in
// `app/actions/agents.ts` and `lib/ws/handlers/agents.ts`.

import type { ProfileEnvVar } from "@/lib/types/http";
import type { AgentProfile, AgentProfilePayload, CLIFlag } from "@/lib/types/agent-profile";
import { agentProfileId } from "@/lib/types/ids";

type RawProfile = Partial<AgentProfilePayload> & Partial<AgentProfile> & Record<string, unknown>;

function pickString(raw: RawProfile, camel: string, snake: string, fallback = ""): string {
  const value = raw[camel] ?? raw[snake];
  return typeof value === "string" ? value : fallback;
}

function pickBool(raw: RawProfile, camel: string, snake: string, fallback = false): boolean {
  const value = raw[camel] ?? raw[snake];
  return typeof value === "boolean" ? value : fallback;
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

/**
 * Convert a kanban snake_case payload (or a partially-camelCased one) into
 * the canonical `AgentProfile`. Office-orchestration fields are left
 * undefined — kanban rows do not carry them.
 */
export function normalizeAgentProfile(raw: unknown): AgentProfile {
  const profile = (raw ?? {}) as RawProfile;
  return {
    id: agentProfileId(pickString(profile, "id", "id")),
    name: pickString(profile, "name", "name"),
    agentId: pickString(profile, "agentId", "agent_id"),
    agentDisplayName: pickString(profile, "agentDisplayName", "agent_display_name"),
    model: pickString(profile, "model", "model"),
    mode: (profile.mode as string | undefined) ?? undefined,
    configOptions: pickConfigOptions(profile),
    allowIndexing: pickBool(profile, "allowIndexing", "allow_indexing"),
    autoApprove: pickBool(profile, "autoApprove", "auto_approve"),
    cliFlags: pickFlags(profile),
    envVars: pickEnvVars(profile),
    cliPassthrough: pickBool(profile, "cliPassthrough", "cli_passthrough"),
    userModified: (profile.userModified ?? profile.user_modified) as boolean | undefined,
    createdAt: pickString(profile, "createdAt", "created_at"),
    updatedAt: pickString(profile, "updatedAt", "updated_at"),
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
  setPayloadField(payload, "name", profile.name);
  setPayloadField(payload, "agent_id", profile.agentId);
  setPayloadField(payload, "agent_display_name", profile.agentDisplayName);
  setPayloadField(payload, "model", profile.model);
  setPayloadField(payload, "mode", profile.mode);
  setPayloadField(payload, "config_options", profile.configOptions);
  setPayloadField(payload, "allow_indexing", profile.allowIndexing);
  setPayloadField(payload, "auto_approve", profile.autoApprove);
  setPayloadField(payload, "cli_flags", profile.cliFlags);
  setPayloadField(payload, "env_vars", profile.envVars);
  setPayloadField(payload, "cli_passthrough", profile.cliPassthrough);
  setPayloadField(payload, "user_modified", profile.userModified);
  setPayloadField(payload, "created_at", profile.createdAt);
  setPayloadField(payload, "updated_at", profile.updatedAt);
  return payload;
}
