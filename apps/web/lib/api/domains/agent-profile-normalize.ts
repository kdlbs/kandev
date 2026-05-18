// Kanban-side AgentProfile normalizer (ADR 0005, Wave E):
// converts the snake_case payloads served by `/api/v1/agents` and
// `/api/v1/agent-profiles/:id` into the canonical camelCase
// `AgentProfile` shape.
//
// Wired via thin wrappers around the server actions / WS payloads in
// `app/actions/agents.ts` and `lib/ws/handlers/agents.ts`.

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
    allowIndexing: pickBool(profile, "allowIndexing", "allow_indexing"),
    cliFlags: pickFlags(profile),
    cliPassthrough: pickBool(profile, "cliPassthrough", "cli_passthrough"),
    userModified: (profile.userModified ?? profile.user_modified) as boolean | undefined,
    createdAt: pickString(profile, "createdAt", "created_at"),
    updatedAt: pickString(profile, "updatedAt", "updated_at"),
  };
}

/**
 * Inverse of `normalizeAgentProfile` — convert the canonical shape back to
 * a snake_case wire payload for `POST/PATCH` to the kanban endpoints.
 */
export function toAgentProfilePayload(
  profile: Partial<AgentProfile>,
): Partial<AgentProfilePayload> {
  const payload: Partial<AgentProfilePayload> = {};
  if (profile.id !== undefined) payload.id = profile.id;
  if (profile.name !== undefined) payload.name = profile.name;
  if (profile.agentId !== undefined) payload.agent_id = profile.agentId;
  if (profile.agentDisplayName !== undefined) payload.agent_display_name = profile.agentDisplayName;
  if (profile.model !== undefined) payload.model = profile.model;
  if (profile.mode !== undefined) payload.mode = profile.mode;
  if (profile.allowIndexing !== undefined) payload.allow_indexing = profile.allowIndexing;
  if (profile.cliFlags !== undefined) payload.cli_flags = profile.cliFlags;
  if (profile.cliPassthrough !== undefined) payload.cli_passthrough = profile.cliPassthrough;
  if (profile.userModified !== undefined) payload.user_modified = profile.userModified;
  if (profile.createdAt !== undefined) payload.created_at = profile.createdAt;
  if (profile.updatedAt !== undefined) payload.updated_at = profile.updatedAt;
  return payload;
}
