import { useTranslation } from "react-i18next";
import {
  IconActivity,
  IconBuilding,
  IconHistory,
  IconLayoutDashboard,
  IconTopologyStar,
} from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { usePathname } from "@/lib/routing/client-router";
import {
  CONTROL_VIEWS,
  CONTROL_VIEW_LABEL_KEYS,
  controlHref,
  controlView,
} from "@/lib/navigation/integrated-control";
import { cn } from "@/lib/utils";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlCoordinatorConversation } from "./integrated-control-conversation-links";

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

/**
 * The workspace tree stays quiet: names and one coordinator conversation entry
 * each, from the shared control snapshot only. Read problems are reported by
 * the page banner, and refreshing is the page header's job.
 */
function ControlWorkspaceTree({ onNavigate }: { onNavigate?: () => void }) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const view = controlView(usePathname());
  if (!control) return null;
  return (
    <section
      className="space-y-1 border-t pt-3"
      aria-label={t("sidebar:integratedControl.workspaces")}
    >
      <h2 className="px-2 text-xs font-medium text-muted-foreground">
        {t("sidebar:integratedControl.workspaces")}
      </h2>
      {!control.coordinatorEnabled && (
        <p className="px-2 text-xs text-muted-foreground">
          {t("sidebar:integratedControl.coordinatorsOff")}
        </p>
      )}
      {control.data?.workspaces.map((workspace) => {
        const coordinators = control.data?.byWorkspace[workspace.id]?.items ?? [];
        return (
          <div
            key={workspace.id}
            className="min-w-0"
            data-testid={`control-workspace-${workspace.id}`}
          >
            <p className="truncate px-2 py-1.5 text-sm font-medium">{workspace.name}</p>
            {coordinators.length > 0 && (
              <div className="ml-3 border-l pl-2">
                {coordinators.map((coordinator) => (
                  <ControlCoordinatorConversation
                    key={coordinator.id}
                    workspaceId={workspace.id}
                    coordinator={coordinator}
                    named={coordinators.length > 1}
                    canManage={hasScope(workspace.scopes, SCOPE.workspaceManage)}
                    view={view}
                    onNavigate={onNavigate}
                  />
                ))}
              </div>
            )}
          </div>
        );
      })}
      {control.data?.workspaces.length === 0 && (
        <p className="px-2 text-sm text-muted-foreground">
          {t("sidebar:integratedControl.noWorkspaces")}
        </p>
      )}
    </section>
  );
}
