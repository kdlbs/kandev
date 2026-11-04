import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import type { TaskNavigationIdentity } from "@/lib/state/task-navigation-reads";

export class TaskSessionAdmissionError extends Error {
  constructor(readonly reason: "unavailable" | "read_failed") {
    super();
  }
}

export function assertRequestedSession(
  identity: TaskNavigationIdentity,
  taskId: string,
  sessionId?: string,
) {
  if (!sessionId) return;
  if (identity.sessionListUnavailable) throw new TaskSessionAdmissionError("read_failed");
  if (
    !identity.allSessionsResponse.sessions.some(
      (session) => session.id === sessionId && session.task_id === taskId,
    )
  ) {
    throw new TaskSessionAdmissionError("unavailable");
  }
}

export function TaskSessionUnavailable({ retry }: { retry?: () => void }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex h-full flex-col items-center justify-center gap-3 p-4"
      data-testid="task-session-unavailable"
    >
      <p role="alert" className="text-sm text-muted-foreground">
        {t(retry ? "task:sessionHistoryUnavailable" : "task:workflowMovePreviewUnknownSession")}
      </p>
      {retry && (
        <Button variant="outline" onClick={retry}>
          {t("task:retry")}
        </Button>
      )}
    </div>
  );
}
