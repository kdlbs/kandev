import { compareTimestamps } from "@/lib/state/slices/settings/types";
import type { AgentProfile } from "@/lib/types/agent-profile";

const EDITABLE_FIELDS = [
  "name",
  "model",
  "fallbackModel",
  "autoFallback",
  "mode",
  "configOptions",
  "allowIndexing",
  "autoApprove",
  "cliFlags",
  "commandPrefix",
  "envVars",
  "cliPassthrough",
  "enabled",
  "dynamic",
] as const satisfies readonly (keyof AgentProfile)[];

type EditableProfile = Pick<AgentProfile, (typeof EDITABLE_FIELDS)[number]>;

function canonicalize(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalize);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>)
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([key, entry]) => [key, canonicalize(entry)]),
  );
}

function editableSnapshot(profile: AgentProfile): EditableProfile {
  return Object.fromEntries(
    EDITABLE_FIELDS.map((field) => [field, profile[field]]),
  ) as EditableProfile;
}

export function sameEditableProfile(left: AgentProfile, right: AgentProfile): boolean {
  return (
    JSON.stringify(canonicalize(editableSnapshot(left))) ===
    JSON.stringify(canonicalize(editableSnapshot(right)))
  );
}

function incomingIsNewer(incoming: AgentProfile, baseline: AgentProfile): boolean {
  const timestampOrder = compareTimestamps(incoming.updatedAt, baseline.updatedAt);
  if (timestampOrder !== 0) return timestampOrder > 0;
  return !sameEditableProfile(incoming, baseline);
}

export type ProfileReconciliationInput = {
  previous: AgentProfile;
  incoming: AgentProfile;
  draft: AgentProfile;
  saved: AgentProfile;
  submitted?: AgentProfile | null;
  conflicted: boolean;
};

export type ProfileReconciliationResult = {
  draft: AgentProfile;
  saved: AgentProfile;
  conflicted: boolean;
  kind: "ignored" | "own-acknowledgement" | "clean-adopted" | "external-conflict";
};

/**
 * Reconciles one authoritative profile snapshot with an open local editor.
 * The helper never merges competing field values: a dirty draft remains local
 * and the newest saved snapshot becomes the baseline for explicit recovery.
 */
export function reconcileAgentProfileSnapshot({
  previous,
  incoming,
  draft,
  saved,
  submitted,
  conflicted,
}: ProfileReconciliationInput): ProfileReconciliationResult {
  if (incoming.id !== previous.id || incoming.id !== saved.id) {
    return { draft, saved, conflicted, kind: "ignored" };
  }

  const ownAcknowledgement = Boolean(submitted && sameEditableProfile(incoming, submitted));
  if (!incomingIsNewer(incoming, saved)) {
    return { draft, saved, conflicted, kind: "ignored" };
  }

  if (ownAcknowledgement) {
    return {
      draft: sameEditableProfile(draft, submitted!) ? incoming : draft,
      saved: incoming,
      conflicted,
      kind: "own-acknowledgement",
    };
  }

  if (sameEditableProfile(draft, saved)) {
    return { draft: incoming, saved: incoming, conflicted, kind: "clean-adopted" };
  }

  return { draft, saved: incoming, conflicted: true, kind: "external-conflict" };
}

export function isProfileRevisionNewer(incoming: AgentProfile, baseline: AgentProfile): boolean {
  return incomingIsNewer(incoming, baseline);
}
