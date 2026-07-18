"use client";

import { useState, type MouseEvent } from "react";
import { usePathname } from "@/lib/routing/client-router";
import { Button } from "@kandev/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from "@kandev/ui/sheet";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconHome, IconMenu2 } from "@tabler/icons-react";
import { PageTopbar } from "@/components/page-topbar";
import Link from "@/components/routing/app-link";
import { SettingsTree } from "@/components/app-sidebar/sections/settings/settings-tree";
import { useAppStore } from "@/components/state-provider";
import { IntegrationCopyConfigMenu } from "@/components/integrations/integration-copy-config-menu";
import { integrationFromPathname } from "@/components/integrations/integration-copy-config";
import { safeDecodePathSegment } from "@/lib/routing/path";

// Brand/initialism overrides so the derived label matches how the rest of the
// app spells these (e.g. "github" → "GitHub", not "Github"). Anything not
// listed here falls back to dash-aware title-casing of the path segment.
const SEGMENT_LABEL_OVERRIDES: Record<string, string> = {
  "azure-devops": "Azure DevOps",
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
// deepest non-id path segment. /settings → null (the topbar still shows
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
    // edit), not on the listing page itself — the listing page title is
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
  const showIntegrationCopyAction = integrationFromPathname(pathname) !== null;

  if (isAgentDetail) {
    return (
      <SettingsShell
        title="Agent"
        backHref="/settings/agents"
        backLabel="Agents"
        parents={[]}
        showIntegrationCopyAction={showIntegrationCopyAction}
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
      showIntegrationCopyAction={showIntegrationCopyAction}
    >
      {children}
    </SettingsShell>
  );
}

function IntegrationCopyConfigAction() {
  const pathname = usePathname();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const activeId = useAppStore((s) => s.workspaces.activeId);
  const routeWorkspaceId = workspaceIdFromPathname(pathname);
  const selected =
    routeWorkspaceId && workspaces.some((workspace) => workspace.id === routeWorkspaceId)
      ? routeWorkspaceId
      : (activeId ?? workspaces[0]?.id ?? null);
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

function workspaceIdFromPathname(pathname: string): string | null {
  const match = pathname.match(/^\/settings\/workspace\/([^/]+)(?:\/|$)/);
  return safeDecodePathSegment(match?.[1]);
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
  parents: Array<{ label: string; href: string }>;
  showIntegrationCopyAction: boolean;
  children: React.ReactNode;
}) {
  const pathname = usePathname();

  return (
    <TooltipProvider>
      <main className="flex min-h-0 flex-1 flex-col">
        <PageTopbar
          title={title}
          backHref={backHref}
          backLabel={backLabel}
          parents={parents}
          leading={<SettingsMobileMenu pathname={pathname} />}
          className="h-10"
          actions={showIntegrationCopyAction ? <IntegrationCopyConfigAction /> : undefined}
        />
        {/* Scroll the content, not the topbar: min-h-0 lets this flex child
            shrink below its content height so overflow-y-auto can take effect. */}
        <div
          data-testid="settings-scroll-container"
          className="flex min-w-0 min-h-0 flex-1 flex-col gap-4 overflow-y-auto overscroll-contain p-4 pb-20"
        >
          {children}
        </div>
      </main>
    </TooltipProvider>
  );
}
