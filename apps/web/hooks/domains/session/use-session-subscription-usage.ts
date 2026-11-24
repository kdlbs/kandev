import { useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { sessionUsesSubscriptionUsage } from "@/lib/session/subscription-usage";

/**
 * Resolves whether the given session runs on the Antigravity personal
 * subscription path, using the session profile and its snapshot fallback.
 */
export function useSessionSubscriptionUsage(sessionId: string | null | undefined): boolean {
  const session = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId] ?? null) : null,
  );
  const profiles = useAppStore((state) => state.agentProfiles.items);
  return useMemo(() => sessionUsesSubscriptionUsage(session, profiles), [session, profiles]);
}
