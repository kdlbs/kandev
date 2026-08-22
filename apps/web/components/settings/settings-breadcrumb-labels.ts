import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";

/**
 * Labels for a settings path *segment*.
 *
 * This is the fallback half of the breadcrumb: `settings-breadcrumbs.ts` names
 * a page's parents and, for record pages, its title. Everything else — the ~45
 * flat settings pages, plus any route added without a breadcrumb row — is
 * titled from its deepest meaningful URL segment, which is what this module
 * turns into copy.
 *
 * A segment with no entry here title-cases, which stays English in every
 * locale. That is a bug, not a feature, so `segmentTitle` reports it
 * (`fromUrl`) and a test gates on it: see `settings-routes.test.ts`
 * ("every shipped settings route resolves to a translated title").
 */

type Translate = (key: string) => string;

/** A resolved label plus where it came from. */
export type SegmentTitle = {
  label: string;
  /**
   * True when the label is a title-cased URL segment rather than catalog copy
   * or a brand name — i.e. it will read as English in every locale.
   */
  fromUrl: boolean;
};

/**
 * Brand/initialism overrides so the derived label matches how the rest of the
 * app spells these. Locale-invariant by nature, so they are not catalog keys.
 * Integration slugs are NOT listed: those come from `WORKSPACE_INTEGRATIONS`
 * below, the same table the command palette and the route list read.
 */
// i18n-exempt: brand names and initialisms, spelled the same in every locale.
const SEGMENT_LABEL_OVERRIDES: Record<string, string> = {
  mcp: "MCP",
  ui: "UI",
  vscode: "VS Code",
};

/** Integration slug → brand label, from the catalog that owns that mapping. */
const INTEGRATION_LABELS: Record<string, string> = Object.fromEntries(WORKSPACE_INTEGRATIONS);

/**
 * Catalog key per settings path segment. The breadcrumb's page title used to be
 * title-cased straight off the URL, which no lint rule can catch (there is no
 * literal to flag) and no locale can translate — the pseudo-locale QA pass is
 * what surfaced it.
 */
const SEGMENT_LABEL_KEYS: Record<string, string> = {
  about: "system:navAbout",
  agent: "settings:agent",
  agents: "common:agents",
  appearance: "settings:appearance",
  automations: "common:automations",
  browse: "agents:browseAvailableAgents",
  changelog: "common:changelog",
  "data-storage": "system:navDataStorage",
  executor: "common:executor",
  executors: "common:executors",
  "external-mcp": "common:externalMcp",
  "feature-toggles": "system:navFeatureToggles",
  organizations: "orgs:navOrganizations",
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
  status: "common:status",
  system: "common:system",
  "task-behavior": "settings:taskBehavior",
  "terminal-editors": "settings:terminalAndEditors",
  tokens: "settings:tokens",
  updates: "system:navUpdates",
  users: "system:navUsers",
  "utility-agents": "settings:utilityAgents",
  workflows: "workflows:workflows",
  workspace: "common:workspace",
  workspaces: "common:workspaces",
};

/**
 * Full-path overrides for pages whose segment word is scope-ambiguous: the
 * install-wide secrets page is "Global Secrets", while a workspace's secrets
 * tab keeps the plain segment label.
 */
const FULL_PATH_LABEL_KEYS: Record<string, string> = {
  "/settings/secrets": "settings:globalSecrets",
};

/** Segments that identify a record rather than name a page. */
const ID_SEGMENT = /^[0-9a-f-]{8,}$/i;

/**
 * Display name for a path segment: a translated page name, a brand name, or —
 * for an unmapped segment — dash-aware title casing, which stays English.
 */
export function segmentTitle(segment: string, t: Translate): SegmentTitle {
  const key = SEGMENT_LABEL_KEYS[segment];
  if (key) return { label: t(key), fromUrl: false };
  const brand = INTEGRATION_LABELS[segment] ?? SEGMENT_LABEL_OVERRIDES[segment];
  if (brand) return { label: brand, fromUrl: false };
  return { label: titleCaseSegment(segment), fromUrl: true };
}

/** Brand label for an integration slug, or null when the slug is not one. */
export function integrationTitle(slug: string | null): string | null {
  return (slug && INTEGRATION_LABELS[slug]) || null;
}

/**
 * Title for a settings page derived from its deepest non-id path segment.
 * `/settings` → null (the topbar names it "Settings" as the page itself).
 * Id-looking segments are skipped, so `/settings/workspace/<uuid>` resolves to
 * "Workspace" rather than the raw id.
 */
export function deriveSegmentTitle(pathname: string, t: Translate): SegmentTitle | null {
  const fullPathKey = FULL_PATH_LABEL_KEYS[pathname];
  if (fullPathKey) return { label: t(fullPathKey), fromUrl: false };

  const segments = pathname.split("/").filter(Boolean);
  if (segments.length <= 1) return null; // just /settings
  for (let index = segments.length - 1; index >= 1; index--) {
    const segment = segments[index];
    if (ID_SEGMENT.test(segment)) continue;
    return segmentTitle(segment, t);
  }
  return null;
}

function titleCaseSegment(segment: string): string {
  return segment
    .split("-")
    .map((part) => (part.length === 0 ? part : part[0].toUpperCase() + part.slice(1)))
    .join(" ");
}
