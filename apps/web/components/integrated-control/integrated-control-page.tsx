import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { AppNavSheet } from "@/components/navigation/app-nav-sheet";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { CONTROL_VIEW_LABEL_KEYS, controlView } from "@/lib/navigation/integrated-control";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import { useIntegratedControl } from "./integrated-control-provider";
import { IntegratedControlConversation } from "./integrated-control-conversation";
import { IntegratedControlDashboard } from "./integrated-control-dashboards";
import type { ControlView } from "@/lib/navigation/integrated-control";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

export function IntegratedControlPage() {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const { isMobile } = useResponsiveBreakpoint();
  const query = useSearchParams();
  const view = controlView(usePathname());
  const workspaceId = query.get("workspace");
  const coordinatorId = query.get("coordinator");
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
        <Button
          variant="ghost"
          size="sm"
          className="max-md:min-h-11"
          disabled={control.loading}
          onClick={() => void control.refresh()}
        >
          {t("sidebar:integratedControl.refresh")}
        </Button>
      </header>
      <ControlBody
        view={view}
        workspace={workspace}
        coordinator={coordinator}
        hasSelection={Boolean(workspaceId || coordinatorId)}
      />
    </div>
  );
}

function ControlBody({
  view,
  workspace,
  coordinator,
  hasSelection,
}: {
  view: ControlView;
  workspace?: ControlWorkspace;
  coordinator?: Coordinator;
  hasSelection: boolean;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  if (control?.error)
    return (
      <p role="alert" className="p-6 text-sm text-destructive">
        {t("sidebar:integratedControl.loadError")}
      </p>
    );
  if (!control?.data)
    return (
      <p role="status" className="p-6 text-sm">
        {t("common:loading")}
      </p>
    );
  if (!hasSelection) return <IntegratedControlDashboard view={view} />;
  if (workspace && coordinator)
    return <IntegratedControlConversation workspace={workspace} coordinator={coordinator} />;
  return (
    <p role="status" className="p-6 text-sm text-muted-foreground">
      {t("sidebar:integratedControl.conversationUnavailable")}
    </p>
  );
}
