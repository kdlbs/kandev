import Link from "next/link";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import type { AgentRecentTask } from "@/lib/api/domains/office-extended-api";
import { formatShortDate } from "./format-date";

type Props = { tasks: AgentRecentTask[] };

const STATUS_LABEL: Record<string, string> = {
  todo: "Todo",
  in_progress: "In progress",
  in_review: "In review",
  done: "Done",
  blocked: "Blocked",
  cancelled: "Cancelled",
  backlog: "Backlog",
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
  return (
    <Card data-testid="recent-tasks-card">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Recent tasks</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        {tasks.length === 0 ? (
          <p className="text-sm text-muted-foreground">No tasks touched yet.</p>
        ) : (
          <ul className="space-y-1">
            {tasks.map((t) => (
              <li
                key={t.task_id}
                data-testid="recent-task-row"
                data-task-id={t.task_id}
                className="flex items-center justify-between gap-2 px-2 py-1 rounded-md hover:bg-muted/50"
              >
                <Link
                  href={`/office/tasks/${t.task_id}`}
                  className="flex items-center gap-2 min-w-0 flex-1 cursor-pointer"
                >
                  {t.identifier ? (
                    <span className="text-xs font-mono text-muted-foreground shrink-0">
                      {t.identifier}
                    </span>
                  ) : null}
                  <span className="text-sm truncate">{t.title}</span>
                </Link>
                <div className="flex items-center gap-2 shrink-0">
                  <Badge
                    variant={STATUS_VARIANT[t.status] ?? "outline"}
                    className="text-[10px] py-0"
                  >
                    {STATUS_LABEL[t.status] ?? t.status}
                  </Badge>
                  <span className="text-[11px] text-muted-foreground">
                    {formatShortDate(t.last_active_at)}
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
