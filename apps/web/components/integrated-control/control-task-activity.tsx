import type { ControlTaskCount } from "@/hooks/domains/coordinator/control-conversation-lists";
import type { Task } from "@/lib/types/http";
import { formatTaskSessionStateLabel } from "@/lib/ui/state-labels";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { linkToTask, linkToTasks } from "@/lib/links";
import { formatDateTime } from "@/lib/i18n/formats";
import { useIntegratedControl } from "./integrated-control-provider";

/** Workspace reads are a bounded navigation sample, never the admission population. */
export function ControlTaskActivity({ workspaceId }: { workspaceId?: string }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const unknown = t("common:unknown");
  if (!control?.data) return null;
  const { workspaces, conversationsByWorkspace } = control.data;
  return (
    <section className="space-y-3" data-testid="control-task-activity">
      <h2 className="font-medium">{t("sidebar:controlInspection.tasks")}</h2>
      {workspaces
        .filter((workspace) => !workspaceId || workspace.id === workspaceId)
        .map((workspace) => {
          const list = conversationsByWorkspace[workspace.id]?.tasks;
          return (
            <section key={workspace.id} className="space-y-3 rounded-lg border p-4">
              <h3 className="font-medium">{workspace.name}</h3>
              <Link
                className="inline-flex min-h-7 items-center underline [@media(pointer:coarse)]:min-h-11"
                href={linkToTasks(workspace.id)}
              >
                {t("sidebar:controlInspection.openTasks")}
              </Link>
              <p className="text-xs text-muted-foreground">
                {t("sidebar:controlInspection.sample", taskSampleCount(list, unknown))}
              </p>
              {list?.error || !list ? (
                <p role="status">
                  {t(
                    list?.denied
                      ? "sidebar:integratedControl.readDenied"
                      : "sidebar:integratedControl.loadError",
                  )}
                </p>
              ) : (
                <ul className="divide-y">
                  {list.items?.map((task) => (
                    <ControlTaskRow key={task.id} task={task} />
                  ))}
                  {list.total === 0 && (
                    <li className="py-3">{t("sidebar:controlInspection.emptyTasks")}</li>
                  )}
                </ul>
              )}
            </section>
          );
        })}
    </section>
  );
}

function ControlTaskRow({ task }: { task: Task }) {
  const { t } = useTranslation();
  const unknown = t("common:unknown");
  return (
    <li className="space-y-1 py-3">
      <Link
        className="inline-flex min-h-7 max-w-full items-center break-words font-medium underline [@media(pointer:coarse)]:min-h-11"
        href={linkToTask(task.id, {
          sessionId: task.primary_session_id ?? undefined,
        })}
      >
        {task.title}
      </Link>
      <p className="break-all text-xs text-muted-foreground">
        {t("sidebar:controlInspection.session")}: {task.primary_session_id ?? unknown} ·{" "}
        {formatTaskSessionStateLabel(task.primary_session_state) || unknown}
      </p>
      <p className="text-xs text-muted-foreground">
        {t("sidebar:integratedControl.agentProfile")}: {task.primary_agent_name || unknown} ·{" "}
        <time dateTime={task.updated_at}>{formatDateTime(task.updated_at)}</time>
      </p>
    </li>
  );
}

function taskSampleCount(list: ControlTaskCount | undefined, unknown: string) {
  if (!list || list.error) return { shown: unknown, total: unknown };
  return { shown: list.items?.length ?? 0, total: list.total ?? unknown };
}
