/**
 * Registry identity of the Antigravity ACP agent's supported personal path.
 * The backend declares subscription billing metadata for this identity. The
 * selector profile projection does not carry billing_type, so the agent
 * identity is the subscription signal available to task surfaces.
 */
export const ANTIGRAVITY_SUBSCRIPTION_AGENT_ID = "antigravity-acp";

type SubscriptionUsageSession =
  | {
      agent_profile_id?: string | null;
      agent_profile_snapshot?: Record<string, unknown> | null;
    }
  | null
  | undefined;

type SubscriptionUsageProfile = {
  id: string;
  agent_name: string;
};

function snapshotAgentName(session: SubscriptionUsageSession): string | null {
  const snapshot = session?.agent_profile_snapshot;
  const agentID = snapshot?.agent_id;
  if (typeof agentID === "string") return agentID;
  const agentName = snapshot?.agent_name;
  return typeof agentName === "string" ? agentName : null;
}

/**
 * Reports whether a session runs on the Antigravity personal subscription path.
 * The session snapshot is the fallback when the profile list is not hydrated.
 */
export function sessionUsesSubscriptionUsage(
  session: SubscriptionUsageSession,
  profiles: readonly SubscriptionUsageProfile[],
): boolean {
  if (!session) return false;
  const profile = session.agent_profile_id
    ? profiles.find((item) => item.id === session.agent_profile_id)
    : undefined;
  const agentName = profile?.agent_name ?? snapshotAgentName(session);
  return agentName === ANTIGRAVITY_SUBSCRIPTION_AGENT_ID;
}
