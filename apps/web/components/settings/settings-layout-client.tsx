"use client";

import { useCallback, useEffect, useMemo, useState, type MouseEvent } from "react";
import { usePathname, useRouter, useSearchParams } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@kandev/ui/sheet";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconHome, IconMenu2 } from "@tabler/icons-react";
import { PageTopbar } from "@/components/page-topbar";
import Link from "@/components/routing/app-link";
import { SettingsTree } from "@/components/app-sidebar/sections/settings/settings-tree";
import { useAppStore } from "@/components/state-provider";
import { WorkspaceSwitcher } from "@/components/task/workspace-switcher";
import { useWorkspaces } from "@/hooks/domains/workspace/use-workspaces";
import { createQueuedUserSettingsSync } from "@/lib/user-settings-sync";
import { IntegrationCopyConfigMenu } from "@/components/integrations/integration-copy-config-menu";
import { integrationFromPathname } from "@/components/integrations/integration-copy-config";
import { safeDecodePathSegment } from "@/lib/routing/path";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";

const WORKSPACE_SYNC_FAILED_KEY = "kandev:settings:integration-workspace:sync-failed:v1";

// Brand/initialism overrides so the derived label matches how the rest of the
// app spells these (e.g. "github" -> "GitHub", not "Github"). Anything not
// listed here falls back to dash-aware title-casing of the path segment.
const SEGMENT_LABEL_OVERRIDES: Record<string, string> = {
  github: "GitHub",
  jira: "Jira",
  linear: "Linear",
  slack: "Slack",
  mcp: "MCP",
  ui: "UI",
  vscode: "VS Code",
};

function titleCase(segment: string): string {
  if (SEGMENT_LABEL_OVERRIDES[segment]) return SEGMENT_LABEL_OVERRIDES[segment];
  return segment
    .split("-")
    .map((p) => (p.length === 0 ? p : p[0].toUpperCase() + p.slice(1)))
    .join(" ");
}

// Derive the human-readable label for the current /settings sub-page from the
// deepest non-id path segment. /settings -> null (the topbar still shows
// "Settings" as the page itself). UUID-looking segments are skipped so e.g.
// /settings/workspace/<uuid> resolves to "Workspace" not the raw id.
function deriveCurrentPageLabel(pathname: string): string | null {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return null; // just /settings
  for (let i = segments.length - 1; i >= 1; i--) {
    const seg = segments[i];
    if (/^[0-9a-f-]{8,}$/i.test(seg)) continue; // skip ids
    return titleCase(seg);
  }
  return null;
}

// Build the intermediate breadcrumb crumbs between the back link and the
// current page title. For workspace-scoped automation pages, inject an
// "Automations" crumb so the breadcrumb reads e.g.
// Home > Settings > Automations > New.
function deriveParents(pathname: string): Array<{ label: string; href: string }> {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return [];

  const parents: Array<{ label: string; href: string }> = [
    { label: "Settings", href: "/settings" },
  ];

  const automationsMatch = pathname.match(
    /^\/settings\/workspace\/([^/]+)\/automations(?:\/(.+))?/,
  );
  if (automationsMatch && automationsMatch[2]) {
    // Only inject the Automations crumb when we're on a sub-page (new or
    // edit), not on the listing page itself; the listing page title is
    // already "Automations".
    parents.push({
      label: "Automations",
      href: `/settings/workspace/${automationsMatch[1]}/automations`,
    });
  }

  return parents;
}

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";
  const showIntegrationActions = integrationFromPathname(pathname) !== null;

  if (isAgentDetail) {
    return (
      <SettingsShell
        title="Agent"
        backHref="/settings/agents"
        backLabel="Agents"
        parents={[]}
        showIntegrationActions={showIntegrationActions}
      >
        {children}
      </SettingsShell>
    );
  }

  const pageLabel = deriveCurrentPageLabel(pathname);
  const title = pageLabel ?? "Settings";
  const parents = deriveParents(pathname);

  return (
    <SettingsShell
      title={title}
      backHref="/"
      backLabel="Kandev"
      parents={parents}
      showIntegrationActions={showIntegrationActions}
    >
      {children}
    </SettingsShell>
  );
}

// useWorkspaceQueryParamSync keeps the top-right switcher and the `?workspace`
// query param in agreement. On load (or when a shared deep link points at a
// valid workspace), it adopts the query param as the active workspace without
// persisting it as the user's global default. setWorkspaceParam writes the
// param back when the user picks a workspace.
function useWorkspaceQueryParamSync(
  workspaces: Array<{ id: string }>,
  activeId: string | null,
  setActiveWorkspace: (id: string) => void,
) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const paramWorkspace = searchParams.get("workspace");

  useEffect(() => {
    if (!paramWorkspace || paramWorkspace === activeId) return;
    if (!workspaces.some((w) => w.id === paramWorkspace)) return;
    setActiveWorkspace(paramWorkspace);
  }, [paramWorkspace, activeId, workspaces, setActiveWorkspace]);

  const setWorkspaceParam = useCallback(
    (workspaceId: string) => {
      const next = new URLSearchParams(window.location.search);
      next.set("workspace", workspaceId);
      router.replace(`${window.location.pathname}?${next.toString()}`, { scroll: false });
    },
    [router],
  );

  return setWorkspaceParam;
}

function workspaceIdFromPathname(pathname: string): string | null {
  const match = pathname.match(/^\/settings\/workspace\/([^/]+)(?:\/|$)/);
  return safeDecodePathSegment(match?.[1]);
}

function IntegrationActions() {
  const pathname = usePathname();
  const { items: workspaces, activeId } = useWorkspaces();
  const setActiveWorkspace = useAppStore((s) => s.setActiveWorkspace);
  const routeWorkspaceId = workspaceIdFromPathname(pathname);
  const scopedWorkspaceId =
    routeWorkspaceId && workspaces.some((workspace) => workspace.id === routeWorkspaceId)
      ? routeWorkspaceId
      : null;
  const selected = scopedWorkspaceId ?? activeId ?? workspaces[0]?.id ?? null;
  const integration = integrationFromPathname(pathname);
  const showSwitcher = pathname.startsWith("/settings/integrations");
  const persistWorkspace = useMemo(
    () =>
      createQueuedUserSettingsSync<string>(WORKSPACE_SYNC_FAILED_KEY, (workspaceId) => ({
        workspace_id: workspaceId,
      })),
    [],
  );
  const setWorkspaceParam = useWorkspaceQueryParamSync(workspaces, activeId, setActiveWorkspace);

  const onSelect = useCallback(
    (workspaceId: string) => {
      setActiveWorkspace(workspaceId);
      setWorkspaceParam(workspaceId);
      void persistWorkspace(workspaceId);
    },
    [persistWorkspace, setActiveWorkspace, setWorkspaceParam],
  );

  if (!integration || !selected || workspaces.length === 0) return null;

  return (
    <div className="flex min-w-0 items-center gap-2">
      {showSwitcher ? (
        <div
          className="flex min-w-0 items-center gap-2"
          data-testid="integration-workspace-switcher"
        >
          <span className="hidden text-xs whitespace-nowrap text-muted-foreground sm:inline">
            Editing workspace
          </span>
          <WorkspaceSwitcher
            workspaces={workspaces}
            activeWorkspaceId={selected}
            onSelect={onSelect}
          />
        </div>
      ) : null}
      <IntegrationCopyConfigMenu
        slug={integration}
        sourceWorkspaceId={selected}
        workspaces={workspaces}
      />
    </div>
  );
}

function SettingsMobileMenu({ pathname }: { pathname: string }) {
  const [open, setOpen] = useState(false);

  const closeOnLinkClick = (event: MouseEvent<HTMLDivElement>) => {
    if (event.target instanceof Element && event.target.closest("a[href]")) {
      setOpen(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-8 w-8 cursor-pointer md:hidden"
          aria-label="Open settings menu"
          data-testid="settings-mobile-menu-button"
        >
          <IconMenu2 className="h-4 w-4" />
        </Button>
      </SheetTrigger>
      <SheetContent
        side="left"
        className="flex w-80 max-w-[85vw] flex-col overflow-hidden p-0"
        data-testid="settings-mobile-menu"
      >
        <SheetHeader className="border-b px-4 py-3 text-left">
          <SheetTitle>Settings</SheetTitle>
        </SheetHeader>
        <nav className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-3">
          <Link
            href="/"
            className="flex h-10 cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-sm font-medium text-foreground/80 transition-colors hover:bg-muted hover:text-foreground"
            onClick={() => setOpen(false)}
          >
            <IconHome className="h-4 w-4 shrink-0" />
            <span className="truncate">Home</span>
          </Link>
          <div
            className="flex flex-col gap-0.5 [&_a]:min-h-10 [&_a]:text-sm [&_button]:min-h-10 [&_button]:text-sm [&_svg]:h-4 [&_svg]:w-4"
            onClick={closeOnLinkClick}
          >
            <SettingsTree pathname={pathname} />
          </div>
        </nav>
      </SheetContent>
    </Sheet>
  );
}

function SettingsShell({
  title,
  backHref,
  backLabel,
  parents,
  showIntegrationActions,
  children,
}: {
  title: string;
  backHref: string;
  backLabel: string;
  parents: Array<{ label: string; href: string }>;
  showIntegrationActions: boolean;
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  return (
    <TooltipProvider>
      <SettingsSaveProvider key={pathname}>
        <main className="flex min-h-0 flex-1 flex-col">
          <PageTopbar
            title={title}
            backHref={backHref}
            backLabel={backLabel}
            parents={parents}
            leading={<SettingsMobileMenu pathname={pathname} />}
            className="h-10"
            actions={showIntegrationActions ? <IntegrationActions /> : undefined}
          />
          {/* Scroll the content, not the topbar: min-h-0 lets this flex child
              shrink below its content height so overflow-y-auto can take effect. */}
          <div
            data-testid="settings-scroll-container"
            className="flex min-w-0 min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain p-4 pb-[calc(6rem_+_env(safe-area-inset-bottom))]"
          >
            {children}
          </div>
        </main>
      </SettingsSaveProvider>
    </TooltipProvider>
  );
}
