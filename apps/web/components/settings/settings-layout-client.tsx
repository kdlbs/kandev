"use client";

import { usePathname } from "@/lib/routing/client-router";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageShell } from "@/components/page-shell";
import type { ParentCrumb } from "@/components/page-topbar";
import { useAppStore } from "@/components/state-provider";
import { IntegrationCopyConfigMenu } from "@/components/integrations/integration-copy-config-menu";
import { integrationFromPathname } from "@/components/integrations/integration-copy-config";
import { safeDecodePathSegment } from "@/lib/routing/path";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { useSettingsBreadcrumbs } from "@/components/settings/use-settings-breadcrumbs";

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { parents, title } = useSettingsBreadcrumbs(pathname);

  return (
    <SettingsShell
      title={title}
      backHref="/"
      backLabel="Kandev"
      parents={parents}
      showIntegrationCopyAction={integrationFromPathname(pathname) !== null}
    >
      {children}
    </SettingsShell>
  );
}

function IntegrationCopyConfigAction() {
  const pathname = usePathname();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const selected = copySourceWorkspaceId(pathname, workspaces, activeId);
  const integration = integrationFromPathname(pathname);

  if (!integration || !selected || workspaces.length === 0) return null;

  return (
    <div className="flex min-w-0 items-center gap-2">
      <IntegrationCopyConfigMenu
        slug={integration}
        sourceWorkspaceId={selected}
        workspaces={workspaces}
      />
    </div>
  );
}

// Either spelling: `workspaces` is canonical, `workspace` the legacy path the
// route table redirects. Both are matched because `integrationFromPathname`
// matches both — a plural-only parse here left the copy action rendered with no
// routed workspace, silently sourcing from the active one instead.
const WORKSPACE_SCOPED_SETTINGS = /^\/settings\/workspaces?\//;

function workspaceIdFromPathname(pathname: string): string | null {
  const match = pathname.match(/^\/settings\/workspaces?\/([^/]+)(?:\/|$)/);
  return safeDecodePathSegment(match?.[1]);
}

/**
 * Which workspace the copy action reads its configuration *from*.
 *
 * An unscoped `/settings/integrations/<slug>` genuinely means "the active
 * workspace" — the route table redirects it into that workspace's tab — so the
 * active workspace is the right source there.
 *
 * A route that names a workspace is different: if that workspace does not
 * resolve (deleted since the URL was bookmarked, or a malformed segment) then
 * falling back would copy credentials out of a workspace the URL never
 * mentioned, with nothing on screen saying so. There is no safe substitute, so
 * the action stays hidden.
 */
function copySourceWorkspaceId(
  pathname: string,
  workspaces: Array<{ id: string }>,
  activeId: string | null,
): string | null {
  if (WORKSPACE_SCOPED_SETTINGS.test(pathname)) {
    const routed = workspaceIdFromPathname(pathname);
    return routed && workspaces.some((workspace) => workspace.id === routed) ? routed : null;
  }
  return activeId ?? workspaces[0]?.id ?? null;
}

function SettingsShell({
  title,
  backHref,
  backLabel,
  parents,
  showIntegrationCopyAction,
  children,
}: {
  title: string;
  backHref: string;
  backLabel: string;
  parents: ParentCrumb[];
  showIntegrationCopyAction: boolean;
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  // A settings form floats its Save action above the fold and needs room to
  // scroll clear of it. #2466 moved that action from the viewport to this
  // column (`placement="content"`), which brought it closer to the bottom and
  // so shrank the reservation. The index floats only the search field, a third
  // of the height, and the form's padding there is dead scroll below the last
  // row.
  const contentBottomPadding =
    pathname === "/settings"
      ? "pb-[calc(5.25rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]"
      : "pb-[calc(8rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))] md:pb-[calc(5.5rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]";

  return (
    <TooltipProvider>
      {/* The positioning ancestor `placement="content"` resolves against: it
          wraps the topbar and the scroll container, so the floating Save bar
          centres on the settings column rather than on the viewport (which
          would sit it under the sidebar). Upstream used the `<main>` this
          shell used to own; `PageShell` renders no landmark of its own, since
          `AppShell` already owns the page's one `<main>`. */}
      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
        <SettingsSaveProvider key={pathname} placement="content">
          <SettingsTargetProvider>
            <PageShell
              title={title}
              backHref={backHref}
              backLabel={backLabel}
              parents={parents}
              showStatusTrigger={false}
              className="h-10"
              actions={showIntegrationCopyAction ? <IntegrationCopyConfigAction /> : undefined}
              // No hamburger inside Settings: `/settings` renders the menu as a
              // page on a phone, reached through the breadcrumb's Settings crumb
              // (a link only below md — on desktop the sidebar menu is always
              // visible, so the crumb is static text). The home crumb leaves.
              showNavTrigger={false}
              contentTestId="settings-scroll-container"
              contentClassName={`flex flex-col gap-4 overscroll-contain p-4 ${contentBottomPadding}`}
            >
              {children}
            </PageShell>
          </SettingsTargetProvider>
        </SettingsSaveProvider>
      </div>
    </TooltipProvider>
  );
}
