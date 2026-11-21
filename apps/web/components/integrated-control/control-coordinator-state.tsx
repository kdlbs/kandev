import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { ControlCoordinatorList } from "@/hooks/domains/coordinator/use-integrated-control-data";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useSession } from "@/hooks/domains/session/use-session";

type CoordinatorState = "responding" | "talking" | "waiting" | "none" | "unknown";
type SessionObservation = "talking" | "waiting" | "unknown";

const LABELS: Record<Exclude<CoordinatorState, "unknown">, string> = {
  responding: "sidebar:controlSummary.coordinator.responding",
  talking: "sidebar:controlSummary.coordinator.talking",
  waiting: "sidebar:controlSummary.coordinator.waiting",
  none: "sidebar:controlSummary.coordinator.none",
};
const TONES: Record<Exclude<CoordinatorState, "unknown">, string> = {
  responding: "border-warning/40 bg-warning/10 text-warning",
  talking: "border-success/40 bg-success/10 text-success",
  waiting: "border-info/40 bg-info/10 text-info",
  none: "border-border bg-muted text-muted-foreground",
};

function CoordinatorPill({ state }: { state: CoordinatorState | null }) {
  const { t } = useTranslation();
  if (state === null) return <span className="text-muted-foreground">-</span>;
  if (state === "unknown")
    return <span className="text-muted-foreground">{t("common:unknown")}</span>;
  return (
    <span
      className={`inline-flex items-center whitespace-nowrap rounded border px-1.5 py-0.5 text-xs ${TONES[state]}`}
      data-state={state}
    >
      {t(LABELS[state])}
    </span>
  );
}

/** Reuse the session's existing shared read and live subscription for ephemeral conversations. */
function CoordinatorSessionObserver({
  taskId,
  report,
}: {
  taskId: string;
  report: (taskId: string, value: SessionObservation) => void;
}) {
  const { sessions, isLoaded, error } = useTaskSessions(taskId);
  const primary = sessions.find((session) => session.is_primary);
  const { session } = useSession(primary?.id ?? null);
  let observation: SessionObservation = "unknown";
  if (isLoaded && !error) {
    if (!primary) observation = "waiting";
    else if (session)
      observation = ["STARTING", "RUNNING"].includes(session.state) ? "talking" : "waiting";
  }
  useEffect(() => report(taskId, observation), [observation, report, taskId]);
  return null;
}

function CoordinatorPresence({ taskIds }: { taskIds: string[] }) {
  const [observed, setObserved] = useState<Record<string, SessionObservation>>({});
  const report = useCallback((taskId: string, value: SessionObservation) => {
    setObserved((previous) =>
      previous[taskId] === value ? previous : { ...previous, [taskId]: value },
    );
  }, []);
  let state: CoordinatorState = "unknown";
  if (taskIds.some((id) => observed[id] === "talking")) state = "talking";
  else if (taskIds.every((id) => observed[id] === "waiting")) state = "waiting";
  return (
    <>
      <CoordinatorPill state={state} />
      {taskIds.map((id) => (
        <CoordinatorSessionObserver key={id} taskId={id} report={report} />
      ))}
    </>
  );
}

export function WorkspaceCoordinatorIndicator({
  enabled,
  list,
}: {
  enabled: boolean;
  list: ControlCoordinatorList | undefined;
}) {
  if (!enabled) return <CoordinatorPill state={null} />;
  if (!list || list.error) return <CoordinatorPill state="unknown" />;
  if (list.items.some((coordinator) => (coordinator.open_proposals ?? 0) > 0))
    return <CoordinatorPill state="responding" />;
  const taskIds = [
    ...new Set(
      list.items.flatMap((coordinator) =>
        coordinator.conversation_task_id ? [coordinator.conversation_task_id] : [],
      ),
    ),
  ].sort();
  if (taskIds.length === 0) return <CoordinatorPill state="none" />;
  return <CoordinatorPresence key={JSON.stringify(taskIds)} taskIds={taskIds} />;
}
