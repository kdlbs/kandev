import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { PageShell } from "@/components/page-shell";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { useCoordinatorSelection } from "./use-coordinator-selection";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  useCoordinatorWorkspace,
  type CoordinatorWorkspace,
} from "@/hooks/domains/orchestration/use-coordinator-workspace";
import { orchestratorsHref } from "@/lib/api/domains/orchestration-api";
import { CoordinatorTaskList } from "./coordinator-task-list";
import { CoordinatorChat } from "./coordinator-chat";
import { CoordinatorTabs } from "./coordinator-tabs";
import { CoordinatorSelect } from "./coordinator-select";

export function CoordinatorPage({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const owner = useAppStore((s) => s.auth.user?.id);
  const enabled = useAppStore((s) => s.features.orchestration);
  return (
    <PageShell title={t("orchestration:coordinator")} scroll="none">
      {enabled ? (
        <CoordinatorWorkspacePage key={`${owner}:${workspaceId}`} workspaceId={workspaceId} />
      ) : (
        <p className="p-6">{t("orchestration:orchestrationDisabled")}</p>
      )}
    </PageShell>
  );
}
function CoordinatorWorkspacePage({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const { data, error, refresh } = useCoordinatorWorkspace(workspaceId);
  if (error)
    return (
      <div className="p-6 space-y-3" role="alert">
        <p>{t("orchestration:workspaceUnavailable")}</p>
        <Button className="cursor-pointer max-md:min-h-11" onClick={refresh}>
          {t("task:retry")}
        </Button>
      </div>
    );
  if (!data)
    return (
      <p role="status" className="p-6">
        {t("common:loading")}
      </p>
    );
  return <CoordinatorContent catalog={data} />;
}
export function CoordinatorContent({ catalog }: { catalog: CoordinatorWorkspace }) {
  const { t } = useTranslation();
  const { requested, selected, choose } = useCoordinatorSelection(catalog);
  const { isMobile } = useResponsiveBreakpoint();
  const [tab, setTab] = useState("tasks");
  const [showChat, setShowChat] = useState(true);
  return (
    <div data-testid="coordinator-page" className="flex min-h-0 min-w-0 flex-1 flex-col">
      <header className="border-b px-4 py-3 flex flex-wrap items-end gap-3">
        <div className="w-full md:w-auto md:flex-1 min-w-0">
          <h1 className="font-semibold truncate">{catalog.workspace.name}</h1>
          <p className="hidden md:block text-xs text-muted-foreground">
            {t("orchestration:coordinatorHint")}
          </p>
        </div>
        <div className="min-w-0 flex-1 md:flex-none md:w-56">
          <CoordinatorSelect
            label={t("orchestration:selectCoordinator")}
            value={selected || "none"}
            onChange={choose}
            options={[
              { id: "none", name: t("orchestration:chooseCoordinator") },
              ...catalog.assignments,
            ]}
            testId="coordinator-selector"
          />
        </div>
        <Link
          href={orchestratorsHref(catalog.workspace.id)}
          className="underline text-xs md:text-sm cursor-pointer max-md:min-h-11 inline-flex items-center"
        >
          {t("orchestration:configureOrchestrator")}
        </Link>
        {!isMobile && (
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer"
            onClick={() => setShowChat(!showChat)}
          >
            {t(showChat ? "orchestration:hideChat" : "orchestration:showChat")}
          </Button>
        )}
      </header>
      {catalog.assignments.length === 0 && (
        <p className="px-4 py-3 text-sm">{t("orchestration:noOrchestrators")}</p>
      )}
      {requested && !selected && (
        <p role="alert" className="px-4 py-3 text-sm">
          {t("orchestration:selectionUnavailable")}
        </p>
      )}
      {isMobile && <CoordinatorTabs tab={tab} setTab={setTab} />}
      <div className="flex flex-1 min-h-0 min-w-0">
        <div
          id="coordinator-panel-chat"
          role={isMobile ? "tabpanel" : undefined}
          aria-labelledby={isMobile ? "coordinator-tab-chat" : undefined}
          hidden={isMobile ? tab !== "chat" : !showChat}
          className="min-h-0 min-w-0 flex-1 flex flex-col [&[hidden]]:hidden"
        >
          <CoordinatorChat catalog={catalog} selected={selected} />
        </div>
        <aside
          id="coordinator-panel-tasks"
          role={isMobile ? "tabpanel" : undefined}
          aria-labelledby={isMobile ? "coordinator-tab-tasks" : undefined}
          hidden={isMobile && tab !== "tasks"}
          className="min-h-0 min-w-0 flex flex-col flex-1 md:flex-none md:w-[44%] md:max-w-2xl md:border-l [&[hidden]]:hidden"
        >
          <CoordinatorTaskList catalog={catalog} selected={selected} />
        </aside>
      </div>
    </div>
  );
}
export { resolveCoordinatorSelection } from "./use-coordinator-selection";
