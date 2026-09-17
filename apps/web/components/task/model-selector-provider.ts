import type { TaskSession } from "@/lib/types/http";

type SessionIdentity = Pick<TaskSession, "agent_profile_id" | "agent_profile_snapshot">;

export function resolveModelSelectorAgentName(
  session: SessionIdentity | null,
  profiles: ReadonlyArray<{ id: string; agent_name: string }>,
): string | null {
  const snapshotName = session?.agent_profile_snapshot?.agent_name;
  if (typeof snapshotName === "string" && snapshotName.trim()) return snapshotName;
  return profiles.find((profile) => profile.id === session?.agent_profile_id)?.agent_name || null;
}
