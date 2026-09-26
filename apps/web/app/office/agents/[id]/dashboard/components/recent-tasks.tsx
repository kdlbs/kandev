import Link from "@/components/routing/app-link";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { AgentRecentTask } from "@/lib/api/domains/office-extended-api";
import { formatShortDate } from "./format-date";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

type Props = { tasks: AgentRecentTask[] };

/** `?? status` keeps an unknown wire value visible rather than blank. */
function statusLabel(t: TFunction, status: string): string {
  const key = STATUS_LABEL_KEYS[status];
  return key ? t(key) : status;
}

// Catalog keys, not copy — module scope freezes a `t()` at the boot locale. The
// record keys are the wire task-status values.
//
// `in_progress` / `in_review` point at the sentence-case keys this dashboard's
// own chart legend already uses, not the title-case `statusInProgress` the board
// and filters use — the two registers are different English and stay separate.
const STATUS_LABEL_KEYS: Record<string, string> = {
  todo: "office:statusTodo",
  in_progress: "office:inProgress",
  in_review: "office:inReview",
  done: "office:statusDone",
  blocked: "office:statusBlocked",
  cancelled: "office:statusCancelled",
  backlog: "office:statusBacklog",
};

const STATUS_VARIANT: Record<string, "default" | "secondary" | "outline" | "destructive"> = {
  done: "default",
  in_progress: "secondary",
  in_review: "secondary",
  blocked: "destructive",
  cancelled: "outline",
  backlog: "outline",
  todo: "outline",
};

export function RecentTasks({ tasks }: Props) {
  const { t } = useTranslation();
  return (
    <Card data-testid="recent-tasks-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{t("office:agentRecentTasks")}</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("office:noTasksTouchedYet")}</p>
        ) : (
          // `task`, not `t`: the map parameter used to shadow the translate
          // function, which is why the status badge could not be localized.
          <ul className="space-y-1">
            {tasks.map((task) => (
              <li
                key={task.task_id}
                data-testid="recent-task-row"
                data-task-id={task.task_id}
                className="flex items-center justify-between gap-2 px-2 py-1 rounded-md hover:bg-muted/50"
              >
                <Link
                  href={`/office/tasks/${task.task_id}`}
                  className="flex items-center gap-2 min-w-0 flex-1 cursor-pointer"
                >
                  {task.identifier ? (
                    <span className="text-xs font-mono text-muted-foreground shrink-0">
                      {task.identifier}
                    </span>
                  ) : null}
                  <span className="text-sm truncate">{task.title}</span>
                </Link>
                <div className="flex items-center gap-2 shrink-0">
                  <Badge
                    variant={STATUS_VARIANT[task.status] ?? "outline"}
                    className="text-[10px] py-0"
                  >
                    {statusLabel(t, task.status)}
                  </Badge>
                  <span className="text-[11px] text-muted-foreground">
                    {formatShortDate(task.last_active_at)}
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
