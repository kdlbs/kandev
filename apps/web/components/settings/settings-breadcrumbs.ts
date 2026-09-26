import { settingsMenuOwnerOf } from "@/components/app-sidebar/sections/settings/settings-menu-sections";
import type { ParentCrumb } from "@/components/page-topbar";
import { safeDecodePathSegment } from "@/lib/routing/path";
import { PLUGINS_SETTINGS_HREF, WORKSPACES_SETTINGS_HREF } from "@/lib/settings-discovery/catalog";
import { deriveSegmentTitle, segmentTitle } from "./settings-breadcrumb-labels";

/**
 * The settings breadcrumb, declared per route.
 *
 * The chain is assembled from three sources, each owning exactly one question:
 *
 *   1. "Settings"      — always first; a link only on a phone, where the
 *                        sidebar that owns the settings menu is hidden.
 *   2. The owning page  — `settingsMenuOwnerOf`, i.e. the same sidebar row that
 *                        highlights for this route. Reading the menu rather
 *                        than re-deriving it is what keeps the crumb and the
 *                        highlighted row from disagreeing, and it covers routes
 *                        with no row of their own (plugin sub-pages, the legacy
 *                        `/settings/workspace/<id>` spellings).
 *   3. `SETTINGS_CRUMB_ROUTES` below — the depth *inside* that page, plus where
 *                        the page title comes from.
 *
 * Anything with no row in (3) falls back to titling itself off its deepest URL
 * segment (`settings-breadcrumb-labels.ts`). That fallback is right for the
 * flat pages, which are their menu row, and wrong for a record page — so
 * `titleFromUrlSegment` reports when it fired and a test gates on it.
 *
 * This replaced a hand-maintained `if` chain with a branch per record type. The
 * chain is why `…/automations/<id>` read "Automations › Automations" (the id was
 * skipped and the title landed back on the section) and why every executor page
 * sat directly under "Settings" (no branch existed).
 */

type Translate = (key: string) => string;

/**
 * Path params captured from a crumb route pattern, percent-decoded. A segment
 * that fails to decode is `null`, never a raw undecodable value: a crumb whose
 * href needs it renders as static text instead of linking somewhere broken.
 */
export type CrumbParams = Readonly<Record<string, string | null>>;

/**
 * Runtime label lookups the settings layout supplies — record names from the
 * store, mostly. Each reads whatever it needs out of the matched params and
 * returns null when it cannot resolve (not loaded yet, deleted, or belonging to
 * a different parent than the URL names).
 */
export type CrumbValues = {
  workspaceName: (params: CrumbParams) => string | null;
  agentDisplayName: (params: CrumbParams) => string | null;
  agentProfileName: (params: CrumbParams) => string | null;
  automationName: (params: CrumbParams) => string | null;
  executorName: (params: CrumbParams) => string | null;
  executorProfileName: (params: CrumbParams) => string | null;
  /** "New <type> Profile" for the create-profile route. */
  executorTypeTitle: (params: CrumbParams) => string | null;
  integrationTitle: (params: CrumbParams) => string | null;
  pluginName: (params: CrumbParams) => string | null;
};

export type CrumbValueName = keyof CrumbValues;

/**
 * What to show while a runtime value has not resolved, or never will. `key` is
 * catalog copy ("Workspace"); `param` title-cases that captured path segment,
 * for values that ARE identifiers — a plugin id, which no catalog can name.
 *
 * A crumb must never fall back to `{ segment: true }`: that reads the *page's*
 * deepest segment, so a parent crumb would end up labelled with its child's
 * name.
 */
export type CrumbFallback = { key: string } | { param: string };

export type CrumbLabel =
  /** A catalog key, translated at render. */
  | { key: string }
  /** A runtime value, with what to show until it resolves. */
  | { value: CrumbValueName; fallback: CrumbFallback }
  /** The deepest non-id URL segment, via the segment label catalog. */
  | { segment: true };

export type CrumbSpec = {
  label: CrumbLabel;
  /**
   * Path template whose `:param` placeholders are filled from the matched
   * params. Omitted — or unresolvable — renders the crumb as static text.
   */
  href?: string;
  phoneOnlyLink?: boolean;
};

export type SettingsCrumbRoute = {
  pattern: RegExp;
  /** Param name per capture group, in order. */
  params: readonly string[];
  /** Crumbs between the owning page and the page title. */
  crumbs: readonly CrumbSpec[];
  title: CrumbLabel;
};

export type SettingsBreadcrumbs = {
  parents: ParentCrumb[];
  title: string;
  /**
   * True when the title is a title-cased URL segment rather than catalog copy,
   * a brand name or a record name — i.e. it stays English in every locale.
   * Read by the coverage gate in `src/settings-routes.test.ts`; the layout
   * itself ignores it.
   */
  titleFromUrlSegment: boolean;
};

const SETTINGS_HREF = "/settings";
const SETTINGS_KEY = "common:settings";
const WORKSPACE_KEY = "common:workspace";
const EXECUTOR_KEY = "common:executor";
const PROFILE_KEY = "common:profile";
const INTEGRATIONS_KEY = "common:integrations";
const AUTOMATIONS_KEY = "common:automations";

const WORKSPACE_HREF = `${WORKSPACES_SETTINGS_HREF}/:workspaceId`;
const WORKSPACE_INTEGRATIONS_HREF = `${WORKSPACE_HREF}/integrations`;
const WORKSPACE_AUTOMATIONS_HREF = `${WORKSPACE_HREF}/automations`;
// Singular `/settings/executor/<id>` is an executor; plural `/settings/executors`
// is the page that lists every executor's profiles. Both are live routes.
const EXECUTOR_HREF = "/settings/executor/:executorId";
const PLUGIN_HREF = `${PLUGINS_SETTINGS_HREF}/:pluginId`;

const SEGMENT_TITLE: CrumbLabel = { segment: true };

const WORKSPACE_CRUMB: CrumbSpec = {
  label: { value: "workspaceName", fallback: { key: WORKSPACE_KEY } },
  href: WORKSPACE_HREF,
};
const INTEGRATIONS_CRUMB: CrumbSpec = {
  label: { key: INTEGRATIONS_KEY },
  href: WORKSPACE_INTEGRATIONS_HREF,
};
const AUTOMATIONS_CRUMB: CrumbSpec = {
  label: { key: AUTOMATIONS_KEY },
  href: WORKSPACE_AUTOMATIONS_HREF,
};
const EXECUTOR_CRUMB: CrumbSpec = {
  label: { value: "executorName", fallback: { key: EXECUTOR_KEY } },
  href: EXECUTOR_HREF,
};
const PLUGIN_LABEL: CrumbLabel = { value: "pluginName", fallback: { param: "pluginId" } };
const PLUGIN_CRUMB: CrumbSpec = { label: PLUGIN_LABEL, href: PLUGIN_HREF };

/**
 * Matched in order, first match wins, so a longer route comes before the
 * shorter one it would otherwise be swallowed by (`/executor/new` before
 * `/executor/<id>`, `/agents/browse` before `/agents/<name>`).
 *
 * A crumb only links to a page that exists. The group index paths
 * (`/settings/preferences`, `/settings/system`) redirect to a sibling, so they
 * get no crumb — a crumb that bounces you somewhere else is worse than none,
 * and on desktop the sidebar already shows the group.
 */
export const SETTINGS_CRUMB_ROUTES: readonly SettingsCrumbRoute[] = [
  // ── Workspaces ────────────────────────────────────────────────────────────
  {
    pattern: /^\/settings\/workspaces\/([^/]+)$/,
    params: ["workspaceId"],
    crumbs: [],
    title: { value: "workspaceName", fallback: { key: WORKSPACE_KEY } },
  },
  {
    pattern: /^\/settings\/workspaces\/([^/]+)\/integrations$/,
    params: ["workspaceId"],
    crumbs: [WORKSPACE_CRUMB],
    title: { key: INTEGRATIONS_KEY },
  },
  {
    pattern: /^\/settings\/workspaces\/([^/]+)\/integrations\/([^/]+)$/,
    params: ["workspaceId", "integrationSlug"],
    crumbs: [WORKSPACE_CRUMB, INTEGRATIONS_CRUMB],
    title: { value: "integrationTitle", fallback: { key: INTEGRATIONS_KEY } },
  },
  {
    pattern: /^\/settings\/workspaces\/([^/]+)\/automations$/,
    params: ["workspaceId"],
    crumbs: [WORKSPACE_CRUMB],
    title: { key: AUTOMATIONS_KEY },
  },
  {
    pattern: /^\/settings\/workspaces\/([^/]+)\/automations\/new$/,
    params: ["workspaceId"],
    crumbs: [WORKSPACE_CRUMB, AUTOMATIONS_CRUMB],
    title: { key: "automations:newAutomation" },
  },
  {
    pattern: /^\/settings\/workspaces\/([^/]+)\/automations\/([^/]+)$/,
    params: ["workspaceId", "automationId"],
    crumbs: [WORKSPACE_CRUMB, AUTOMATIONS_CRUMB],
    title: { value: "automationName", fallback: { key: "common:automation" } },
  },
  {
    // Tabs of the workspace page. Their own segment names them; they need the
    // row only so the workspace-name crumb appears.
    pattern: /^\/settings\/workspaces\/([^/]+)\/(?:repositories|workflows|secrets)$/,
    params: ["workspaceId"],
    crumbs: [WORKSPACE_CRUMB],
    title: SEGMENT_TITLE,
  },
  // ── Agents ────────────────────────────────────────────────────────────────
  {
    // The install catalogue, not an agent — so it must precede the agent route.
    pattern: /^\/settings\/agents\/browse$/,
    params: [],
    crumbs: [],
    title: SEGMENT_TITLE,
  },
  {
    // No agent-name crumb: a saved agent has no page of its own (the route
    // redirects to the index), so its profiles sit directly under Agents.
    pattern: /^\/settings\/agents\/([^/]+)\/profiles\/([^/]+)$/,
    params: ["agentName", "agentProfileId"],
    crumbs: [],
    title: { value: "agentProfileName", fallback: { key: PROFILE_KEY } },
  },
  {
    pattern: /^\/settings\/agents\/([^/]+)$/,
    params: ["agentName"],
    crumbs: [],
    title: { value: "agentDisplayName", fallback: { key: "settings:agent" } },
  },
  // ── Executors ─────────────────────────────────────────────────────────────
  {
    pattern: /^\/settings\/executors\/new\/([^/]+)$/,
    params: ["executorType"],
    crumbs: [],
    title: { value: "executorTypeTitle", fallback: { key: "executors:newProfile" } },
  },
  {
    pattern: /^\/settings\/executors\/ssh\/([^/]+)$/,
    params: ["executorId"],
    crumbs: [],
    title: { value: "executorName", fallback: { key: EXECUTOR_KEY } },
  },
  {
    pattern: /^\/settings\/executors\/k8s\/([^/]+)$/,
    params: ["executorId"],
    crumbs: [],
    title: { value: "executorName", fallback: { key: EXECUTOR_KEY } },
  },
  {
    pattern: /^\/settings\/executors\/([^/]+)$/,
    params: ["executorProfileId"],
    crumbs: [],
    title: { value: "executorProfileName", fallback: { key: PROFILE_KEY } },
  },
  {
    pattern: /^\/settings\/executor\/new$/,
    params: [],
    crumbs: [],
    title: SEGMENT_TITLE,
  },
  {
    pattern: /^\/settings\/executor\/([^/]+)\/profile\/([^/]+)$/,
    params: ["executorId", "executorProfileId"],
    crumbs: [EXECUTOR_CRUMB],
    title: { value: "executorProfileName", fallback: { key: PROFILE_KEY } },
  },
  {
    pattern: /^\/settings\/executor\/([^/]+)$/,
    params: ["executorId"],
    crumbs: [],
    title: { value: "executorName", fallback: { key: EXECUTOR_KEY } },
  },
  // ── Plugins ───────────────────────────────────────────────────────────────
  {
    pattern: /^\/settings\/plugins\/([^/]+)$/,
    params: ["pluginId"],
    crumbs: [],
    title: PLUGIN_LABEL,
  },
  {
    // Sub-pages a plugin registered itself. Their depth is the plugin's own, so
    // only the plugin crumb is known here.
    pattern: /^\/settings\/plugins\/([^/]+)\/.+$/,
    params: ["pluginId"],
    crumbs: [PLUGIN_CRUMB],
    title: SEGMENT_TITLE,
  },
];

export type MatchedCrumbRoute = { route: SettingsCrumbRoute; params: CrumbParams };

export function matchSettingsCrumbRoute(pathname: string): MatchedCrumbRoute | null {
  for (const route of SETTINGS_CRUMB_ROUTES) {
    const match = pathname.match(route.pattern);
    if (!match) continue;
    const params: Record<string, string | null> = {};
    route.params.forEach((name, index) => {
      params[name] = safeDecodePathSegment(match[index + 1]);
    });
    return { route, params };
  }
  return null;
}

type ResolveContext = {
  pathname: string;
  params: CrumbParams;
  t: Translate;
  values: CrumbValues;
};

const NO_PARAMS: CrumbParams = {};

/**
 * Fill a crumb href template. Returns null when a placeholder has no usable
 * value, so the caller renders static text rather than a link to nowhere. Ids
 * are re-encoded: params arrive decoded, for lookups and display.
 */
function fillCrumbHref(template: string, params: CrumbParams): string | null {
  let href = template;
  for (const [name, value] of Object.entries(params)) {
    const token = `:${name}`;
    if (!href.includes(token)) continue;
    if (value === null) return null;
    href = href.replace(token, encodeURIComponent(value));
  }
  return href.includes(":") ? null : href;
}

type ResolvedLabel = { label: string; fromUrl: boolean };

function resolveSegment(context: ResolveContext): ResolvedLabel {
  return (
    deriveSegmentTitle(context.pathname, context.t) ?? {
      label: context.t(SETTINGS_KEY),
      fromUrl: false,
    }
  );
}

function resolveFallback(fallback: CrumbFallback, context: ResolveContext): ResolvedLabel {
  if ("key" in fallback) return { label: context.t(fallback.key), fromUrl: false };
  const param = context.params[fallback.param];
  if (param) return segmentTitle(param, context.t);
  return resolveSegment(context);
}

function resolveCrumbLabel(label: CrumbLabel, context: ResolveContext): ResolvedLabel {
  if ("key" in label) return { label: context.t(label.key), fromUrl: false };
  if ("segment" in label) return resolveSegment(context);
  const resolved = context.values[label.value](context.params);
  if (resolved) return { label: resolved, fromUrl: false };
  return resolveFallback(label.fallback, context);
}

/**
 * Build the breadcrumb for a `/settings/*` pathname: the parent crumbs
 * `PageTopbar` renders plus the current page title.
 */
export function resolveSettingsBreadcrumbs(
  pathname: string,
  t: Translate,
  values: CrumbValues,
): SettingsBreadcrumbs {
  if (pathname.split("/").filter(Boolean).length <= 1) {
    // `/settings` itself: the page IS Settings, so it has no parents.
    return { parents: [], title: t("settings:settings"), titleFromUrlSegment: false };
  }

  const matched = matchSettingsCrumbRoute(pathname);
  const context: ResolveContext = { pathname, params: matched?.params ?? NO_PARAMS, t, values };
  const parents: ParentCrumb[] = [
    { label: t(SETTINGS_KEY), href: SETTINGS_HREF, phoneOnlyLink: true },
  ];

  const owner = settingsMenuOwnerOf(pathname);
  if (owner) parents.push({ label: t(owner.labelKey), href: owner.href });

  for (const crumb of matched?.route.crumbs ?? []) {
    const href = crumb.href ? fillCrumbHref(crumb.href, context.params) : null;
    parents.push({
      label: resolveCrumbLabel(crumb.label, context).label,
      ...(href ? { href } : {}),
      ...(crumb.phoneOnlyLink ? { phoneOnlyLink: true } : {}),
    });
  }

  const title = resolveCrumbLabel(matched?.route.title ?? SEGMENT_TITLE, context);
  return { parents, title: title.label, titleFromUrlSegment: title.fromUrl };
}
