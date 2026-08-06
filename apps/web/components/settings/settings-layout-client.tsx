"use client";

import { usePathname } from "@/lib/routing/client-router";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { PageShell } from "@/components/page-shell";
import { useAppStore } from "@/components/state-provider";
import { IntegrationCopyConfigMenu } from "@/components/integrations/integration-copy-config-menu";
import { integrationFromPathname } from "@/components/integrations/integration-copy-config";
import { safeDecodePathSegment } from "@/lib/routing/path";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { SettingsTargetProvider } from "@/components/settings/settings-target-provider";
import { useTranslation } from "react-i18next";

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
  browse: "agents:browseAvailableAgents",
  changelog: "common:changelog",
  "data-storage": "system:navDataStorage",
  executor: "settings:executor",
  executors: "common:executors",
  "external-mcp": "common:externalMcp",
  integrations: "common:integrations",
  "keyboard-shortcuts": "settings:keyboardShortcuts",
  layouts: "settings:layouts",
  new: "settings:new",
  notifications: "settings:notifications",
  plugins: "common:plugins",
  preferences: "settings:preferences",
  profiles: "executors:profiles",
  prompts: "common:prompts",
  repositories: "sidebar:repositories",
  secrets: "settings:secrets",
  security: "settings:security",
  system: "common:system",
  "task-behavior": "settings:taskBehavior",
  "terminal-editors": "settings:terminalAndEditors",
  tokens: "settings:tokens",
  "utility-agents": "settings:utilityAgents",
  "voice-mode": "settings:voiceMode",
  workflows: "workflows:workflows",
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

// Full-path overrides for pages whose segment word is scope-ambiguous: the
// install-wide secrets page is "Global Secrets", while a workspace's secrets
// tab keeps the plain segment label.
const FULL_PATH_LABEL_KEYS: Record<string, string> = {
  "/settings/secrets": "settings:globalSecrets",
};

// Derive the human-readable label for the current /settings sub-page from the
// deepest non-id path segment. /settings → null (the topbar still shows
// "Settings" as the page itself). UUID-looking segments are skipped so e.g.
// /settings/workspace/<uuid> resolves to "Workspace" not the raw id.
function deriveCurrentPageLabel(pathname: string, t: (key: string) => string): string | null {
  if (FULL_PATH_LABEL_KEYS[pathname]) return t(FULL_PATH_LABEL_KEYS[pathname]);
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
// current page title. The "Settings" crumb is static text on desktop (the
// sidebar owns the settings menu there) but stays a link to the settings
// index on phones, where it is the only way back to the menu. Detail pages
// (workspaces, agents) get a clickable crumb for their list page, and their
// sub-pages an additional name crumb back to the detail root.
function deriveParents(
  pathname: string,
  workspaceName: string | null,
  t: (key: string) => string,
): Array<{ label: string; href?: string; phoneOnlyLink?: boolean }> {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return [];

  const parents: Array<{ label: string; href?: string; phoneOnlyLink?: boolean }> = [
    { label: t("common:settings"), href: "/settings", phoneOnlyLink: true },
  ];

  const workspaceDetail = pathname.match(/^\/settings\/workspaces\/([^/]+)(\/.+)?$/);
  if (workspaceDetail) {
    parents.push({ label: t("common:workspaces"), href: "/settings/workspaces" });
    if (workspaceDetail[2]) {
      parents.push({
        label: workspaceName ?? t("common:workspace"),
        href: `/settings/workspaces/${workspaceDetail[1]}`,
      });
    }
  }

  const agentDetail = pathname.match(/^\/settings\/agents\/([^/]+)(\/.+)?$/);
  if (agentDetail) {
    // No agent-name crumb: saved agents have no page of their own (the route
    // redirects to the index), so profile and browse pages sit directly under
    // Agents.
    parents.push({ label: t("common:agents"), href: "/settings/agents" });
  }

  const automationsMatch = pathname.match(
    /^\/settings\/workspaces\/([^/]+)\/automations(?:\/(.+))?/,
  );
  if (automationsMatch && automationsMatch[2]) {
    // Only inject the Automations crumb when we're on a sub-page (new or
    // edit), not on the listing page itself — the listing page title is
    // already "Automations".
    parents.push({
      label: t("common:automations"),
      href: `/settings/workspaces/${automationsMatch[1]}/automations`,
    });
  }

  return parents;
}

// The record identities a settings pathname can carry, parsed in one place.
// `isRoot` separates a record's own page from its sub-pages: only the former is
// titled with the record's name.
type RecordRoute = {
  workspaceId: string | null;
  workspaceIsRoot: boolean;
  agentName: string | null;
  agentIsRoot: boolean;
  profileId: string | null;
};

function matchRecordRoute(pathname: string): RecordRoute {
  const workspace = pathname.match(/^\/settings\/workspaces\/([^/]+)(\/.+)?$/);
  // Agent routes are keyed by agent name; `browse` is the catalog index rather
  // than a saved agent, so it carries no record name.
  const agent = pathname.match(/^\/settings\/agents\/([^/]+)(\/.+)?$/);
  const profile = pathname.match(/^\/settings\/agents\/[^/]+\/profiles\/([^/]+)$/);

  return {
    workspaceId: safeDecodePathSegment(workspace?.[1]),
    workspaceIsRoot: Boolean(workspace) && !workspace?.[2],
    agentName: agent && agent[1] !== "browse" ? safeDecodePathSegment(agent[1]) : null,
    agentIsRoot: Boolean(agent) && !agent?.[2],
    profileId: safeDecodePathSegment(profile?.[1]),
  };
}

// A record's own page is titled with the record's name, not the list label;
// everything else falls back to the derived page label, then to "Settings".
function resolveTitle(
  route: RecordRoute,
  names: { workspace: string | null; agent: string | null; profile: string | null },
  pageLabel: string | null,
  t: (key: string) => string,
): string {
  if (route.workspaceIsRoot && names.workspace) return names.workspace;
  if (route.agentName && route.agentIsRoot) return names.agent ?? t("settings:agent");
  return names.profile ?? pageLabel ?? t("settings:settings");
}

export function SettingsLayoutClient({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const availableAgents = useAppStore((s) => s.availableAgents.items);
  const settingsAgents = useAppStore((s) => s.settingsAgents.items);

  const route = matchRecordRoute(pathname);
  const workspaceName = route.workspaceId
    ? (workspaces.find((workspace) => workspace.id === route.workspaceId)?.name ?? null)
    : null;
  const agentDisplayName = route.agentName
    ? (availableAgents.find((agent) => agent.name === route.agentName)?.display_name ?? null)
    : null;
  const profileName = route.profileId
    ? (settingsAgents
        .flatMap((agent) => agent.profiles)
        .find((profile) => profile.id === route.profileId)?.name ?? null)
    : null;

  const names = { workspace: workspaceName, agent: agentDisplayName, profile: profileName };

  return (
    <SettingsShell
      title={resolveTitle(route, names, deriveCurrentPageLabel(pathname, t), t)}
      backHref="/"
      backLabel="Kandev"
      parents={deriveParents(pathname, workspaceName, t)}
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
  parents: Array<{ label: string; href?: string; phoneOnlyLink?: boolean }>;
  showIntegrationCopyAction: boolean;
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  // A settings form floats its Save action above the fold and needs room to
  // scroll clear of it. The index floats only the search field, which is a
  // third of that — the same padding there is dead scroll below the last row.
  const contentBottomPadding =
    pathname === "/settings"
      ? "pb-[calc(5.25rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]"
      : "pb-[calc(11rem_+_env(safe-area-inset-bottom)_+_var(--app-status-bar-height))]";

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
    </TooltipProvider>
  );
}
