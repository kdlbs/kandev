import { useTranslation } from "react-i18next";
import { useState } from "react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useNativeWorkspaceTasks } from "@/hooks/domains/coordinator/use-control-native";
import type { OverviewTaskFilter } from "@/lib/types/control-overview";
import { linkToTask, linkToTasks } from "@/lib/links";
import { formatDateTime } from "@/lib/i18n/formats";
import type { NativeArchive, NativeTask } from "@/lib/types/control-native";
import { sessionStateLabel, taskStateLabel } from "./overview-format";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlReadState } from "./control-read-state";
import { ControlTaskActivity } from "./control-task-activity";

const FILTER_CHIPS: { filter: OverviewTaskFilter; labelKey: string }[] = [
  { filter: "problems", labelKey: "office:overviewFilterProblems" },
  { filter: "active", labelKey: "common:taskStateInProgress" },
  { filter: "sessions", labelKey: "sidebar:controlRestore.processRunning" },
  { filter: "queued", labelKey: "chat:queuedMessages" },
  { filter: "completed", labelKey: "office:overviewCompleted24h" },
  { filter: "hold", labelKey: "office:projectStatusOnHold" },
  { filter: "all", labelKey: "office:all" },
];

export function ControlWorkspaceTaskList({
  workspaceId,
  filter,
  onFilter,
  supported,
}: {
  workspaceId: string;
  filter: OverviewTaskFilter;
  onFilter: (filter: OverviewTaskFilter) => void;
  supported: boolean;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const [limit, setLimit] = useState(50);
  const [archive, setArchive] = useState<NativeArchive>(filter === "completed" ? "all" : "active");
  const list = useNativeWorkspaceTasks(
    workspaceId,
    filter,
    archive,
    filter === "problems" ? -1 : limit,
    supported ? control?.data : undefined,
  );
  if (!supported)
    return (
      <div className="border-t p-3">
        <ControlTaskActivity workspaceId={workspaceId} />
      </div>
    );
  const rows = list.data?.tasks;
  return (
    <div className="space-y-3 border-t p-3">
      <div className="flex flex-wrap gap-1.5">
        {FILTER_CHIPS.map((chip) => (
          <Button
            key={chip.filter}
            size="sm"
            className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
            variant={chip.filter === filter ? "secondary" : "ghost"}
            aria-pressed={chip.filter === filter}
            onClick={() => {
              setLimit(50);
              onFilter(chip.filter);
            }}
          >
            {t(chip.labelKey)}
          </Button>
        ))}
      </div>
      <div className="flex flex-wrap gap-1.5" data-testid="control-task-archive">
        {(["active", "archived", "all"] as const).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={value === archive ? "secondary" : "ghost"}
            aria-pressed={value === archive}
            onClick={() => {
              setLimit(50);
              setArchive(value);
            }}
          >
            {t(`sidebar:controlNative.archive.${value}`)}
          </Button>
        ))}
      </div>
      <ControlReadState {...list} />
      {list.status === "ready" && !rows && <ControlReadState status="unsupported" />}
      <ul className="divide-y">
        {rows?.map((row) => (
          <NativeTaskRow key={row.task_id} row={row} />
        ))}
      </ul>
      {rows && (
        <p className="text-xs text-muted-foreground">
          {t("office:overviewShowingOf", { shown: rows.length, total: list.data?.total })}
        </p>
      )}
      <ShowAllTasks
        shown={rows?.length}
        total={list.data?.total}
        limit={limit}
        onShowAll={() => setLimit(-1)}
      />
      <Link
        className="inline-flex min-h-7 cursor-pointer items-center text-sm underline [@media(pointer:coarse)]:min-h-11"
        href={linkToTasks(workspaceId)}
      >
        {t("sidebar:controlInspection.openTasks")}
      </Link>
    </div>
  );
}

function ShowAllTasks({
  shown,
  total,
  limit,
  onShowAll,
}: {
  shown?: number;
  total?: number;
  limit: number;
  onShowAll: () => void;
}) {
  const { t } = useTranslation();
  if (shown === undefined || total === undefined || total <= shown || limit === -1) return null;
  return (
    <Button
      variant="link"
      className="cursor-pointer [@media(pointer:coarse)]:min-h-11"
      onClick={onShowAll}
      data-testid="control-workspace-show-all"
    >
      {t("office:overviewShowAll")}
    </Button>
  );
}

function NativeTaskRow({ row }: { row: NativeTask }) {
  const { t } = useTranslation();
  return (
    <li key={row.task_id} className="flex flex-wrap gap-x-3 py-2 text-sm">
      <Link
        className="inline-flex min-h-7 cursor-pointer items-center underline [@media(pointer:coarse)]:min-h-11"
        href={linkToTask(row.task_id, row.session_id ? { sessionId: row.session_id } : undefined)}
      >
        {row.title}
      </Link>
      <span>{row.step_name}</span>
      <span>{taskStateLabel(t, row.state)}</span>
      {row.session_state && (
        <span>
          {t("sidebar:controlNative.primarySession")}: {sessionStateLabel(t, row.session_state)}
        </span>
      )}
      <span>
        {t("sidebar:controlRestore.processRunning")}: {row.running_sessions} ·{" "}
        {t("chat:queuedMessages")}: {row.queued_messages}
      </span>
      {row.archived && <span>{t("sidebar:controlNative.archive.archived")}</span>}
      <time dateTime={row.updated_at}>
        {t("sidebar:controlNative.updatedAt")}: {formatDateTime(row.updated_at)}
      </time>
    </li>
  );
}
