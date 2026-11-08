import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import Link from "@/components/routing/app-link";
import { linkToTask } from "@/lib/links";
import type { NativeTask } from "@/lib/types/control-native";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlFilterChips } from "./control-filter-chips";
import { ControlStatusPill } from "./control-status-pill";
import { ControlReadState } from "./control-read-state";
import {
  ATTENTION_SIGNALS,
  sortBySignal,
  taskSignal,
  taskWaitReason,
  type TaskSignal,
} from "./control-signals";
import {
  stageCounts,
  TASK_STAGES,
  taskStage,
  taskStageTotal,
  type TaskStage,
} from "./control-stages";
import { relativeTime, shortId } from "./overview-format";
import type { ControlSignals } from "./use-control-signals";

type BoardFilter =
  | "all"
  | "attention"
  | "running"
  | "waiting"
  | "held"
  | "paused"
  | "queued"
  | "failed"
  | "noSession";
const FILTERS: BoardFilter[] = [
  "all",
  "attention",
  "running",
  "waiting",
  "held",
  "paused",
  "queued",
  "failed",
  "noSession",
];
const FILTER_LABELS: Record<BoardFilter, string> = {
  all: "office:all",
  attention: "sidebar:controlSummary.board.attention",
  running: "sidebar:controlSummary.state.running",
  waiting: "sidebar:controlSummary.state.waiting",
  held: "sidebar:controlSummary.stage.held",
  paused: "sidebar:controlSummary.state.paused",
  queued: "sidebar:controlSummary.state.queued",
  failed: "sidebar:controlSummary.state.failed",
  noSession: "sidebar:controlSummary.state.noSession",
};
const STAGE_LABELS: Record<TaskStage, string> = {
  work: "sidebar:controlSummary.stage.work",
  review: "sidebar:controlSummary.stage.review",
  held: "sidebar:controlSummary.stage.held",
  completed: "sidebar:controlSummary.stage.completed",
  unknown: "common:unknown",
};

type StageFilter = TaskStage | "all";

function matches(filter: BoardFilter, signal: TaskSignal) {
  if (filter === "all") return true;
  if (filter === "attention") return ATTENTION_SIGNALS.includes(signal);
  return filter === signal;
}

type Row = { task: NativeTask; signal: TaskSignal };

/** Every unarchived task in one table, ranked so the rows that need a person come first. */
export function ControlTaskBoard({ signals }: { signals: ControlSignals }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const [filter, setFilter] = useState<BoardFilter>("all");
  const [stage, setStage] = useState<StageFilter>("all");
  const names = Object.fromEntries(
    (control?.data?.workspaces ?? []).map((workspace) => [workspace.id, workspace.name]),
  );
  const profiles = useMemo(() => {
    const byTask = new Map<string, string>();
    for (const lists of Object.values(control?.data?.conversationsByWorkspace ?? {}))
      for (const task of lists.tasks?.items ?? [])
        if (task.primary_agent_name) byTask.set(task.id, task.primary_agent_name);
    return byTask;
  }, [control?.data]);
  const rows = useMemo(() => {
    const repeating = new Set(signals.repeats.keys());
    return sortBySignal(
      signals.tasks.map((task): Row => ({ task, signal: taskSignal(task, repeating) })),
    );
  }, [signals.tasks, signals.repeats]);
  const counts = Object.fromEntries(
    FILTERS.map((value) => [value, rows.filter((row) => matches(value, row.signal)).length]),
  ) as Record<BoardFilter, number>;
  const stages = stageCounts(signals.tasks);
  const visible = rows.filter(
    (row) => matches(filter, row.signal) && (stage === "all" || taskStage(row.task) === stage),
  );
  const failures = signals.taskReads.data?.filter((row) => row.status !== "ready") ?? [];
  return (
    <section className="space-y-2" data-testid="control-task-board">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-sm font-semibold">{t("sidebar:controlInspection.tasks")}</h2>
        <ControlFilterChips
          chips={FILTERS.map((value) => ({
            value,
            label: t(FILTER_LABELS[value]),
            count: counts[value],
          }))}
          selected={filter}
          onSelect={setFilter}
          testId="control-task-filter"
        />
      </div>
      <ControlFilterChips
        chips={[
          { value: "all", label: t("office:all"), count: taskStageTotal(stages) },
          ...TASK_STAGES.map((value) => ({
            value,
            label: t(STAGE_LABELS[value]),
            count: stages[value],
          })),
        ]}
        selected={stage}
        onSelect={setStage}
        testId="control-task-stage"
      />
      {signals.taskReads.status !== "ready" && <ControlReadState {...signals.taskReads} />}
      {failures.map((row) => (
        <div key={row.id} className="flex items-center text-xs text-muted-foreground">
          {names[row.id] ?? row.id}
          <ControlReadState {...row} />
        </div>
      ))}
      <TaskTable
        rows={visible}
        names={names}
        profiles={profiles}
        empty={visible.length === 0 && signals.taskReads.status === "ready"}
      />
    </section>
  );
}

function TaskTable({
  rows,
  names,
  profiles,
  empty,
}: {
  rows: Row[];
  names: Record<string, string>;
  profiles: Map<string, string>;
  empty: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="rounded-lg border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("sidebar:controlSummary.column.state")}</TableHead>
            <TableHead>{t("sidebar:controlSummary.column.workspace")}</TableHead>
            <TableHead>{t("sidebar:controlSummary.column.task")}</TableHead>
            <TableHead>{t("sidebar:controlSummary.column.step")}</TableHead>
            <TableHead>{t("sidebar:controlSummary.column.reason")}</TableHead>
            <TableHead>{t("sidebar:integratedControl.agentProfile")}</TableHead>
            <TableHead>{t("sidebar:controlInspection.session")}</TableHead>
            <TableHead>{t("sidebar:controlSummary.column.lastChange")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map(({ task, signal }) => (
            <TableRow key={task.task_id} data-testid="control-task-row" data-signal={signal}>
              <TableCell>
                <ControlStatusPill state={signal} />
                <div className="mt-1 text-xs text-muted-foreground">{task.state}</div>
              </TableCell>
              <TableCell className="whitespace-nowrap">
                {names[task.workspace_id] ?? task.workspace_id}
              </TableCell>
              <TableCell className="max-w-80 whitespace-normal break-words">
                <Link
                  className="underline-offset-4 hover:underline"
                  href={linkToTask(
                    task.task_id,
                    task.session_id ? { sessionId: task.session_id } : undefined,
                  )}
                >
                  {task.title}
                </Link>
              </TableCell>
              <TableCell className="whitespace-nowrap text-muted-foreground">
                {task.step_name || "-"}
              </TableCell>
              <TableCell className="max-w-64 whitespace-normal text-xs text-muted-foreground">
                {taskWaitReason(task)
                  ? t(`sidebar:controlSummary.reason.${taskWaitReason(task)}`)
                  : "-"}
              </TableCell>
              <TableCell className="max-w-40 truncate text-muted-foreground">
                {profiles.get(task.task_id) ?? "-"}
              </TableCell>
              <TableCell className="font-mono text-xs">
                {task.session_id ? (
                  <span title={task.session_id}>{shortId(task.session_id)}</span>
                ) : (
                  <span className="text-muted-foreground">
                    {t("sidebar:controlSummary.state.noSession")}
                  </span>
                )}
              </TableCell>
              <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                <time dateTime={task.updated_at}>{relativeTime(task.updated_at)}</time>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {empty && (
        <p className="px-4 py-3 text-sm text-muted-foreground">
          {t("sidebar:controlInspection.emptyTasks")}
        </p>
      )}
    </div>
  );
}
