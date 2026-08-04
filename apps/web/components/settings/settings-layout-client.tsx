"use client";

import { usePathname } from "@/lib/routing/client-router";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageShell } from "@/components/page-shell";
import { SettingsTree } from "@/components/app-sidebar/sections/settings/settings-tree";
import { useAppStore } from "@/components/state-provider";
import { IntegrationCopyConfigMenu } from "@/components/integrations/integration-copy-config-menu";
import { integrationFromPathname } from "@/components/integrations/integration-copy-config";
import { safeDecodePathSegment } from "@/lib/routing/path";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";

// Brand/initialism overrides so the derived label matches how the rest of the
// app spells these (e.g. "github" → "GitHub", not "Github"). Anything not
// listed here falls back to dash-aware title-casing of the path segment.
const SEGMENT_LABEL_OVERRIDES: Record<string, string> = {
  "azure-devops": "Azure DevOps",
  github: "GitHub",
  jira: "Jira",
  linear: "Linear",
  mcp: "MCP",
  ui: "UI",
  vscode: "VS Code",
};

/**
 * Catalog key per settings path segment. The breadcrumb's page title used to be
 * title-cased straight off the URL, which no lint rule can catch (there is no
 * literal to flag) and no locale can translate — the pseudo-locale QA pass is
 * what surfaced it. `SEGMENT_LABEL_OVERRIDES` above stays for brand names, which
 * are the same in every language.
 */
const SEGMENT_LABEL_KEYS: Record<string, string> = {
  agent: "settings:agent",
  agents: "common:agents",
  appearance: "settings:appearance",
  automations: "common:automations",
  changelog: "common:changelog",
  "changes-panel": "settings:changesPanel",
  "chat-input": "settings:chatInput",
  editors: "settings:editors",
  executor: "settings:executor",
  executors: "common:executors",
  "external-mcp": "common:externalMcp",
  general: "settings:general",
  integrations: "common:integrations",
  "keyboard-shortcuts": "settings:keyboardShortcuts",
  layouts: "settings:layouts",
  "message-queue": "system:messageQueueTitle",
  new: "settings:new",
  notifications: "settings:notifications",
  plugins: "common:plugins",
  prompts: "common:prompts",
  "resource-metrics": "settings:resourceMetrics",
  secrets: "settings:secrets",
  security: "settings:security",
  shell: "common:shell",
  sprites: "settings:sprites",
  system: "common:system",
  "task-actions": "settings:taskActions",
  terminal: "settings:terminal",
  tokens: "settings:tokens",
  "utility-agents": "settings:utilityAgents",
  "voice-mode": "settings:voiceMode",
  workspace: "common:workspace",
  workspaces: "common:workspaces",
};

/**
 * Display name for a path segment: a translated page name, a brand override, or
 * (for an unmapped route) dash-aware title casing, which stays English.
 */
function segmentLabel(segment: string, t: (key: string) => string): string {
  if (SEGMENT_LABEL_KEYS[segment]) return t(SEGMENT_LABEL_KEYS[segment]);
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
function deriveCurrentPageLabel(pathname: string, t: (key: string) => string): string | null {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return null; // just /settings
  for (let i = segments.length - 1; i >= 1; i--) {
    const seg = segments[i];
    if (/^[0-9a-f-]{8,}$/i.test(seg)) continue; // skip ids
    return segmentLabel(seg, t);
  }
  return null;
}

// Build the intermediate breadcrumb crumbs between the back link and the
// current page title. Workspace-scoped pages include the routed workspace so
// the breadcrumb identifies which workspace owns the settings being edited.
function deriveParents(
  pathname: string,
  workspaceName: string | null,
  t: (key: string) => string,
): Array<{ label: string; href: string }> {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return [];

  const parents: Array<{ label: string; href: string }> = [
    { label: t("common:settings"), href: "/settings" },
  ];

  const workspaceId = workspaceIdFromPathname(pathname);
  if (workspaceId) {
    parents.push({
      label: workspaceName ?? t("common:workspace"),
      href: `/settings/workspace/${encodeURIComponent(workspaceId)}`,
    });
  }

  const automationsMatch = pathname.match(
    /^\/settings\/workspace\/([^/]+)\/automations(?:\/(.+))?/,
  );
  if (automationsMatch && automationsMatch[2]) {
    // Only inject the Automations crumb when we're on a sub-page (new or
    // edit), not on the listing page itself — the listing page title is
    // already "Automations".
    parents.push({
      label: t("common:automations"),
      href: `/settings/workspace/${automationsMatch[1]}/automations`,
    });
  }

  return parents;
}

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const isAgentDetail = pathname.startsWith("/settings/agents/") && pathname !== "/settings/agents";
  const showIntegrationCopyAction = integrationFromPathname(pathname) !== null;

  if (isAgentDetail) {
    return (
      <SettingsShell
        title={t("settings:agent")}
        backHref="/settings/agents"
        backLabel={t("common:agents")}
        parents={[]}
        showIntegrationCopyAction={showIntegrationCopyAction}
      >
        {children}
      </SettingsShell>
    );
  }

  const pageLabel = deriveCurrentPageLabel(pathname, t);
  const title = pageLabel ?? t("settings:settings");
  const routeWorkspaceId = workspaceIdFromPathname(pathname);
  const workspaceName =
    workspaces.find((workspace) => workspace.id === routeWorkspaceId)?.name ?? null;
  const parents = deriveParents(pathname, workspaceName, t);

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

function workspaceIdFromPathname(pathname: string): string | null {
  const match = pathname.match(/^\/settings\/workspace\/([^/]+)(?:\/|$)/);
  return safeDecodePathSegment(match?.[1]);
}

/**
 * The settings tree as the nav sheet's page section. The wrapper compacts the
 * sidebar-styled tree to the sheet's touch-row sizing; link clicks close the
 * sheet via AppNavSheet's own handler.
 */
function SettingsPageNav({ pathname }: { pathname: string }) {
  return (
    <div className="flex flex-col gap-0.5 [&_a]:min-h-10 [&_a]:text-sm [&_button]:min-h-10 [&_button]:text-sm [&_svg]:h-4 [&_svg]:w-4">
      <SettingsTree pathname={pathname} />
    </div>
  );
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
      <SettingsSaveProvider key={pathname}>
        <SettingsTargetProvider>
          <PageShell
            title={title}
            backHref={backHref}
            backLabel={backLabel}
            parents={parents}
            showStatusTrigger={false}
            className="h-10"
            actions={showIntegrationCopyAction ? <IntegrationCopyConfigAction /> : undefined}
            pageNav={<SettingsPageNav pathname={pathname} />}
            contentTestId="settings-scroll-container"
            contentClassName="flex flex-col gap-4 overscroll-contain p-4 pb-[calc(11rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]"
          >
            {children}
          </PageShell>
        </SettingsTargetProvider>
      </SettingsSaveProvider>
    </TooltipProvider>
  );
}
