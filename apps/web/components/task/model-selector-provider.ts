import type { TaskSession } from "@/lib/types/http";

type SessionIdentity = Pick<TaskSession, "agent_profile_id" | "agent_profile_snapshot">;

/** Snapshot identity takes precedence over the session's editable logical profile. */
export function resolveModelSelectorAgentName(
  session: SessionIdentity | null,
  profiles: ReadonlyArray<{ id: string; agent_id: string; agent_name: string }>,
): string | null {
  const snapshotName = session?.agent_profile_snapshot?.agent_name;
  if (typeof snapshotName === "string" && snapshotName.trim()) return snapshotName;
  const snapshotAgentId = session?.agent_profile_snapshot?.agent_id;
  if (typeof snapshotAgentId === "string" && snapshotAgentId.trim()) {
    const snapshotProfile = profiles.find((profile) => profile.agent_id === snapshotAgentId);
    if (snapshotProfile?.agent_name) return snapshotProfile.agent_name;
  }
  return profiles.find((profile) => profile.id === session?.agent_profile_id)?.agent_name || null;
}
