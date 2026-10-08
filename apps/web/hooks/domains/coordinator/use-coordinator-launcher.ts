import { useEffect, useState } from "react";
import { ApiError } from "@/lib/api/client";
import { getCoordinator, type Coordinator } from "@/lib/api/domains/coordinator-api";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useSession } from "@/hooks/domains/session/use-session";
import { isSessionWorking } from "@/lib/session-working";

export type CoordinatorLauncherState = {
  /** null until the launcher's own GET resolves, or after it fails; a
   *  failure never falls back to a stale value. */
  coordinator: Coordinator | null;
  loading: boolean;
  busy: boolean;
  /** True on a 404 from the launcher's own GET; resets on the next
   *  coordinator id. Distinguished from any other failure, which just
   *  leaves the launcher not busy
   *  (docs/specs/coordinator/system-design/copilot-popover.md#ask-about-this). */
  gone: boolean;
};

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

function usePrimaryTaskSessionId(taskId: string | null): string | null {
  const { sessions } = useTaskSessions(taskId);
  if (!taskId) return null;
  return (sessions.find((session) => session.is_primary) ?? sessions[0])?.id ?? null;
}

/**
 * The launcher's own coordinator read and busy-state resolution
 * (docs/specs/coordinator/system-design/copilot-popover.md
 * #coordinator-read, #launcher-busy-state). Issues `getCoordinator` once
 * when the viewed coordinator resolves or changes, discarding a response
 * that arrives after the id changed again; never calls the conversation
 * route and never resumes or restores a session.
 *
 * `routeSessionId` is the latest successful conversation-route response's
 * `session_id` for this coordinator in this page, owned by the open
 * sequence; when set it takes priority over the GET's
 * `conversation_task_id`, so a screen visit does not itself create a
 * conversation task or session.
 */
export function useCoordinatorLauncher(
  workspaceId: string,
  coordinatorId: string | null,
  routeSessionId: string | null,
): CoordinatorLauncherState {
  const [coordinator, setCoordinator] = useState<Coordinator | null>(null);
  const [loading, setLoading] = useState(false);
  const [gone, setGone] = useState(false);

  useEffect(() => {
    setCoordinator(null);
    setGone(false);
    if (!coordinatorId) return;
    let cancelled = false;
    setLoading(true);
    getCoordinator(workspaceId, coordinatorId)
      .then((result) => {
        if (!cancelled) setCoordinator(result);
      })
      .catch((error) => {
        // The popover body separately surfaces the open sequence's own GET
        // failure; the launcher just stays not busy, except a 404, which it
        // reports so the copilot store can drop a gone coordinator's entry.
        if (!cancelled && isNotFound(error)) setGone(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, coordinatorId]);

  const conversationTaskId = routeSessionId ? null : (coordinator?.conversation_task_id ?? null);
  const primarySessionId = usePrimaryTaskSessionId(conversationTaskId);
  const sessionId = routeSessionId ?? primarySessionId;
  const { session } = useSession(sessionId);

  return { coordinator, loading, busy: isSessionWorking(session), gone };
}
