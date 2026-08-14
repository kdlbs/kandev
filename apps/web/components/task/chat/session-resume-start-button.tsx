"use client";

import { useCallback, useState } from "react";
import { IconPlayerPlay } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { TaskSessionState } from "@/lib/types/http";
import { useAppStore } from "@/components/state-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildResumeRequest } from "@/lib/services/session-launch-helpers";
import { isLaunchStateRegression } from "@/lib/session-state";

/**
 * "Start agent" affordance for a resume-skipped session (prevent-auto-start-on-
 * open preference): the session exists with history (its agent process is not
 * running) and the open-time auto-resume was skipped, so the session stayed
 * stopped. Clicking resumes the agent.
 *
 * Rendered in the message-list footer for resume-skipped sessions (the
 * task-description start button only handles never-started CREATED sessions). The resume
 * is NOT gated by the preference — it is the explicit manual action.
 */
export function SessionResumeStartButton({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation();
  const [isStarting, setIsStarting] = useState(false);
  const session = useAppStore((state) => state.taskSessions.items[sessionId] ?? null);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const setResumeSkipped = useAppStore((state) => state.setResumeSkipped);

  const handleStart = useCallback(async () => {
    if (!session) return;
    setIsStarting(true);
    try {
      const { request } = buildResumeRequest(session.task_id, sessionId);
      const response = await launchSession(request);
      if (response.success && response.state) {
        // Hydrate the launch state (commonly STARTING) so the button hides
        // immediately and the UI reflects the in-flight resume instead of
        // staying stopped and clickable for repeated resumes. Never apply it
        // over a newer live state: a WS RUNNING/FAILED transition can land
        // before the launch response resolves, and the delayed STARTING must
        // not hide a running agent or a failure's recovery affordances.
        if (!isLaunchStateRegression(session.state, response.state)) {
          setTaskSession({
            id: session.id,
            task_id: session.task_id,
            state: response.state as TaskSessionState,
            started_at: session.started_at ?? "",
            updated_at: session.updated_at ?? "",
          });
        }
      }
      // Only confirmed RUNNING clears the resume-skipped marker: a resume
      // response commonly reports STARTING (launch accepted, agent starting),
      // and the WS RUNNING transition clears it later. Failed launches keep
      // the button as a retry affordance.
      if (response.state === "RUNNING") {
        setResumeSkipped(sessionId, false);
      }
    } catch (error) {
      console.error("Failed to resume agent:", error);
    } finally {
      setIsStarting(false);
    }
  }, [session, sessionId, setTaskSession, setResumeSkipped]);

  return (
    <div className="flex justify-end mt-1.5">
      <Button
        size="sm"
        variant="default"
        className="cursor-pointer gap-1.5"
        onClick={handleStart}
        disabled={isStarting}
        data-testid="session-resume-start-button"
      >
        <IconPlayerPlay className="h-3.5 w-3.5" />
        {isStarting ? t("task:startingEllipsis") : t("task:startAgent")}
      </Button>
    </div>
  );
}
