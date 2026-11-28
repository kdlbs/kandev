import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useNativeDashboard } from "@/hooks/domains/coordinator/use-control-native";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import type { OverviewTaskFilter } from "@/lib/types/control-overview";
import { workspaceHomeHref } from "@/lib/navigation/workspace-home";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { useIntegratedControl } from "./integrated-control-provider";
import { NativeWorkspaceFigures } from "./control-native-metrics";
import { ControlReadState } from "./control-read-state";
import { ControlWorkspaceSample } from "./control-workspace-sample";
import { ControlWorkspaceTaskList } from "./control-workspace-task-list";

const UNKNOWN_KEY = "common:unknown";

export function ControlWorkspaces({ workspaceId }: { workspaceId?: string }) {
  const control = useIntegratedControl();
  return (
    <div className="grid gap-3" data-testid="control-workspaces">
      {control?.data?.workspaces
        .filter((workspace) => !workspaceId || workspace.id === workspaceId)
        .map((workspace) => (
          <ControlWorkspaceCard key={workspace.id} workspace={workspace} />
        ))}
    </div>
  );
}

function ControlWorkspaceCard({ workspace }: { workspace: ControlWorkspace }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const detail = useNativeDashboard(workspace.id, control?.data);
  const metrics = detail.data;
  const [filter, setFilter] = useState<OverviewTaskFilter | null>(null);
  const taskCount = control?.data?.conversationsByWorkspace[workspace.id]?.tasks;
  return (
    <Card
      className="w-full min-w-0 gap-0 overflow-hidden p-0"
      data-testid="control-workspace-card"
      aria-busy={detail.status === "loading" || Boolean(detail.refreshing)}
    >
      <ControlWorkspaceHeader workspace={workspace} />
      {(detail.status !== "ready" || detail.stale) && <ControlReadState {...detail} />}
      {detail.status === "ready" && !metrics && <ControlReadState status="unsupported" />}
      {metrics && <NativeWorkspaceFigures data={metrics} filter={filter} onFilter={setFilter} />}
      {!metrics && <ControlWorkspaceSample list={taskCount} />}
      <div className="px-4 pb-3 pt-2">
        <Button
          variant="ghost"
          size="sm"
          className="mt-2 cursor-pointer"
          aria-expanded={filter !== null}
          onClick={() => setFilter((current) => (current ? null : "all"))}
        >
          {t(filter ? "office:overviewCollapseDetails" : "office:overviewExpandDetails")}
        </Button>
      </div>
      {filter && (
        <ControlWorkspaceTaskList
          key={filter}
          workspaceId={workspace.id}
          filter={filter}
          onFilter={setFilter}
          supported={Boolean(metrics)}
        />
      )}
    </Card>
  );
}

function ControlWorkspaceHeader({ workspace }: { workspace: ControlWorkspace }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const startupPage = useAppStore((state) => state.userSettings.startupPage);
  const taskCount = control?.data?.conversationsByWorkspace[workspace.id]?.tasks;
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 pt-3">
        <Link
          href={workspaceHomeHref(workspace, startupPage)}
          className="inline-flex min-h-7 cursor-pointer items-center font-medium underline [@media(pointer:coarse)]:min-h-11"
        >
          {workspace.name}
        </Link>
        <span className="text-xs text-muted-foreground">
          {t(
            hasScope(workspace.scopes, SCOPE.workspaceManage)
              ? "sidebar:integratedControl.manageAccess"
              : "sidebar:integratedControl.readAccess",
          )}
        </span>
        <span className="text-sm">
          {t("sidebar:integratedControl.unarchivedTasks")}:{" "}
          {taskCount && !taskCount.error ? (taskCount.total ?? t(UNKNOWN_KEY)) : t(UNKNOWN_KEY)}
        </span>
      </div>
      {workspace.description && (
        <p className="px-4 text-xs text-muted-foreground">{workspace.description}</p>
      )}
    </>
  );
}
