import { useTranslation } from "react-i18next";
import { IconRefresh } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { AppNavSheet } from "@/components/navigation/app-nav-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { CONTROL_VIEW_LABEL_KEYS, controlView } from "@/lib/navigation/integrated-control";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { useIntegratedControl } from "./integrated-control-provider";
import { IntegratedControlConversation } from "./integrated-control-conversation";
import { ControlTaskActivity } from "./control-task-activity";
import { IntegratedControlDashboard } from "./integrated-control-dashboards";
import type { ControlView } from "@/lib/navigation/integrated-control";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ControlConversationLists } from "@/hooks/domains/coordinator/control-conversation-lists";
import { ControlLastRefresh, ControlRefreshBanner } from "./control-refresh-banner";
import { ControlDbBadge } from "./control-db-badge";

function originalControlTask(
  lists: ControlConversationLists | undefined | false,
  coordinator: Coordinator | undefined,
  taskId: string | null,
  coordinatorRequested = false,
) {
  if (coordinatorRequested && !coordinator) return undefined;
  if (!lists) return undefined;
  if (!coordinator) return lists.independent.items.find((item) => item.task_id === taskId)?.task_id;
  return !taskId || taskId === coordinator.conversation_task_id
    ? (coordinator.conversation_task_id ?? undefined)
    : undefined;
}

export function IntegratedControlPage() {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const { isMobile } = useResponsiveBreakpoint();
  const query = useSearchParams();
  const view = controlView(usePathname());
  const workspaceId = query.get("workspace");
  const coordinatorId = query.get("coordinator");
  const taskId = query.get("task");
  const workspace = control?.data?.workspaces.find((item) => item.id === workspaceId);
  const coordinator = workspace
    ? control?.data?.byWorkspace[workspace.id]?.items.find(
        (item) => item.id === coordinatorId && item.workspace_id === workspace.id,
      )
    : undefined;
  if (!control?.active) return null;
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="integrated-control-page">
      <header className="flex min-h-10 shrink-0 items-center gap-2 border-b px-4 py-2">
        {isMobile && <AppNavSheet />}
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-sm font-semibold">
            {t("sidebar:integratedControl.title")} / {t(CONTROL_VIEW_LABEL_KEYS[view])}
          </h1>
          {workspace && coordinator && (
            <p className="truncate text-xs text-muted-foreground">
              {workspace.name} / {coordinator.name}
            </p>
          )}
        </div>
        <ControlDbBadge />
        <ControlLastRefresh />
        <Button
          variant="ghost"
          size="sm"
          className="max-md:min-h-11"
          onClick={() => void control.refresh()}
          data-testid="control-refresh"
        >
          <IconRefresh
            className={cn("size-4", control.loading && "animate-spin")}
            aria-hidden="true"
          />
          {t("sidebar:integratedControl.refresh")}
        </Button>
      </header>
      <ControlRefreshBanner />
      <ControlBody
        view={view}
        workspace={workspace}
        coordinator={coordinator}
        taskId={taskId}
        coordinatorRequested={Boolean(coordinatorId)}
        hasSelection={Boolean(workspaceId || coordinatorId || taskId)}
      />
    </div>
  );
}

function ControlBody({
  view,
  workspace,
  coordinator,
  taskId,
  coordinatorRequested,
  hasSelection,
}: {
  view: ControlView;
  workspace?: ControlWorkspace;
  coordinator?: Coordinator;
  taskId: string | null;
  coordinatorRequested: boolean;
  hasSelection: boolean;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  // A failed first read is reported by the page banner alone.
  if (control?.failure && !control.data) return null;
  if (!control?.data)
    return (
      <p role="status" className="p-6 text-sm">
        {t("common:loading")}
      </p>
    );
  if (!hasSelection) return <IntegratedControlDashboard view={view} />;
  if (workspace && workspaceDashboard(view, coordinatorRequested, taskId))
    return <IntegratedControlDashboard view={view} workspaceId={workspace.id} />;
  if (workspace && !coordinatorRequested && !taskId)
    return (
      <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
        <ControlTaskActivity workspaceId={workspace.id} />
      </div>
    );
  const lists = workspace && control.data.conversationsByWorkspace[workspace.id];
  const originalTaskId = originalControlTask(lists, coordinator, taskId, coordinatorRequested);
  if (workspace && originalTaskId)
    return (
      <IntegratedControlConversation
        workspace={workspace}
        coordinator={coordinator}
        taskId={originalTaskId}
      />
    );
  return (
    <p role="status" className="p-6 text-sm text-muted-foreground">
      {t("sidebar:integratedControl.conversationUnavailable")}
    </p>
  );
}

function workspaceDashboard(
  view: ControlView,
  coordinatorRequested: boolean,
  taskId: string | null,
) {
  return !coordinatorRequested && !taskId && ["workspaces", "history", "providers"].includes(view);
}
