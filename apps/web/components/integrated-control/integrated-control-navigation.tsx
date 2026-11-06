import { useTranslation } from "react-i18next";
import {
  IconActivity,
  IconBuilding,
  IconHistory,
  IconLayoutDashboard,
  IconMessage,
  IconTopologyStar,
} from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { usePathname, useSearchParams } from "@/lib/routing/client-router";
import {
  CONTROL_VIEWS,
  CONTROL_VIEW_LABEL_KEYS,
  controlHref,
  controlView,
} from "@/lib/navigation/integrated-control";
import { cn } from "@/lib/utils";
import { useIntegratedControl } from "./integrated-control-provider";

const icons = [IconLayoutDashboard, IconActivity, IconBuilding, IconTopologyStar, IconHistory];

export function IntegratedControlNavigation({
  collapsed = false,
  scrollable = true,
  onNavigate,
}: {
  collapsed?: boolean;
  scrollable?: boolean;
  onNavigate?: () => void;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const view = controlView(usePathname());
  if (!control?.active) return null;
  return (
    <div
      className={cn(
        "flex min-h-0 flex-col gap-3 p-2",
        scrollable ? "flex-1 overflow-y-auto" : "flex-none",
      )}
      data-testid="integrated-control-navigation"
    >
      <div className="flex flex-col gap-1">
        {CONTROL_VIEWS.map((item, index) => {
          const Icon = icons[index];
          const label = t(CONTROL_VIEW_LABEL_KEYS[item]);
          return (
            <Link
              key={item}
              href={controlHref(item)}
              title={label}
              aria-label={collapsed ? label : undefined}
              aria-current={view === item ? "page" : undefined}
              onNavigated={onNavigate}
              className={cn(
                "flex min-h-7 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
                view === item ? "bg-accent font-medium" : "hover:bg-muted",
              )}
              data-testid={`control-nav-${item}`}
            >
              <Icon className="size-4 shrink-0" aria-hidden />
              {!collapsed && <span>{label}</span>}
            </Link>
          );
        })}
      </div>
      {!collapsed && <ControlWorkspaceTree onNavigate={onNavigate} />}
    </div>
  );
}

function ControlWorkspaceTree({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const query = useSearchParams();
  const view = controlView(usePathname());
  if (!control) return null;
  return (
    <section
      className="space-y-2 border-t pt-3"
      aria-label={t("sidebar:integratedControl.workspaces")}
    >
      <div className="flex items-center justify-between gap-2 px-2">
        <h2 className="text-xs font-medium text-muted-foreground">
          {t("sidebar:integratedControl.workspaces")}
        </h2>
        <Button
          variant="ghost"
          size="sm"
          className="max-md:min-h-11"
          disabled={control.loading}
          onClick={() => void control.refresh()}
        >
          {t("sidebar:integratedControl.refresh")}
        </Button>
      </div>
      {control.loading && (
        <p role="status" className="px-2 text-xs text-muted-foreground">
          {t("common:loading")}
        </p>
      )}
      {control.error && (
        <p role="alert" className="px-2 text-xs text-destructive">
          {t("sidebar:integratedControl.loadError")}
        </p>
      )}
      {control.data?.workspaces.map((workspace) => (
        <details
          key={workspace.id}
          open
          className="min-w-0 rounded-md"
          data-testid={`control-workspace-${workspace.id}`}
        >
          <summary className="cursor-pointer truncate px-2 py-1.5 text-sm font-medium max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
            {workspace.name}
          </summary>
          <div className="ml-3 border-l pl-2">
            {!control.coordinatorEnabled && (
              <p className="p-2 text-xs text-muted-foreground">
                {t("sidebar:integratedControl.coordinatorsOff")}
              </p>
            )}
            {control.data?.byWorkspace[workspace.id]?.error && (
              <p role="alert" className="p-2 text-xs text-destructive">
                {t("sidebar:integratedControl.loadError")}
              </p>
            )}
            {control.data?.byWorkspace[workspace.id]?.items.map((coordinator) => (
              <details key={coordinator.id} open className="min-w-0">
                <summary className="cursor-pointer truncate px-2 py-1.5 text-sm max-md:min-h-11 [@media(pointer:coarse)]:min-h-11">
                  {coordinator.name}
                </summary>
                {coordinator.conversation_task_id ? (
                  <Link
                    href={controlHref(view, workspace.id, coordinator.id)}
                    onNavigated={onNavigate}
                    aria-current={
                      query.get("workspace") === workspace.id &&
                      query.get("coordinator") === coordinator.id
                        ? "page"
                        : undefined
                    }
                    className="ml-3 flex min-h-7 cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-muted aria-[current=page]:bg-accent max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
                    data-testid={`control-conversation-${coordinator.id}`}
                  >
                    <IconMessage className="size-3.5 shrink-0" aria-hidden />
                    {t("sidebar:integratedControl.currentConversation")}
                  </Link>
                ) : (
                  <p className="px-5 py-1.5 text-xs text-muted-foreground">
                    {t("sidebar:integratedControl.noConversation")}
                  </p>
                )}
              </details>
            ))}
          </div>
        </details>
      ))}
      {control.data?.workspaces.length === 0 && (
        <p className="px-2 text-sm text-muted-foreground">
          {t("sidebar:integratedControl.noWorkspaces")}
        </p>
      )}
    </section>
  );
}
