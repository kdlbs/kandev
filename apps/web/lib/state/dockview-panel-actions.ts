import type { DockviewApi, DockviewGroupPanel } from "dockview-react";
import type { ChangeLayer, CommitDetailTarget } from "@/components/task/changes-diff-target";
import { t } from "@/lib/i18n";
import { focusOrAddPanel } from "./dockview-layout-builders";
import { reviewPanelId, type ReviewPanelTarget } from "./dockview-review-panel-id";
import { panelTitle } from "./layout-manager/panel-title";

export type ScrollTarget = {
  sessionId: string;
  messageId: string;
  token: number;
  hostPanelId: string;
};
export type StoreGet = () => {
  api: DockviewApi | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  selectedDiff: { path: string; content?: string } | null;
  scrollTarget?: ScrollTarget | null;
};
export type StoreSet = (
  partial: Partial<{
    selectedDiff: { path: string; content?: string } | null;
    scrollTarget: ScrollTarget | null;
  }>,
) => void;

export type SimplePanelOpts = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
};

/** Add a panel (or focus an existing one with the same id) inside the given group. */
export function addSimplePanel(api: DockviewApi, groupId: string, opts: SimplePanelOpts): void {
  focusOrAddPanel(api, { ...opts, position: { referenceGroup: groupId } });
}

export { reviewPanelId };

/** The browser panel a preview should reuse: the active one, else the first. */
function findBrowserPanel(api: DockviewApi) {
  return (
    (api.activePanel?.api.component === "browser" ? api.activePanel : undefined) ??
    api.panels.find((panel) => panel.api.component === "browser")
  );
}

function openBrowserPanel(api: DockviewApi, centerGroupId: string, url: string): void {
  const browserPanel = findBrowserPanel(api);

  if (browserPanel) {
    browserPanel.api.updateParameters({ url });
    browserPanel.api.setActive();
    return;
  }

  focusOrAddPanel(api, {
    id: `browser:${url}`,
    component: "browser",
    title: t("task:browser"),
    params: { url },
    position: { referenceGroup: centerGroupId },
  });
}

export type SidePanelOpts = { groupId?: string; quiet?: boolean; inCenter?: boolean };
export type ReviewPanelOptions = { groupId?: string };

/**
 * Shared placement logic for a single-instance "side" panel (Plan, a plugin
 * task panel, ...): an explicit `groupId` wins, `inCenter` falls back to the
 * center group, and otherwise the panel opens beside the chat panel. Extracted
 * from `addPlanPanel` so `addPluginPanel` can reuse the identical placement
 * rules instead of re-deriving them.
 */
export function addSidePanel(
  api: DockviewApi,
  centerGroupId: string,
  panel: SimplePanelOpts,
  opts?: SidePanelOpts,
): void {
  const groupId = opts?.groupId ?? (opts?.inCenter ? centerGroupId : undefined);
  const position = groupId
    ? { referenceGroup: groupId }
    : { referencePanel: "chat" as const, direction: "right" as const };
  focusOrAddPanel(api, { ...panel, position }, opts?.quiet ?? false);
}

/**
 * Focus the legacy single-instance panel when it carries the matching key
 * param and the modern keyed panel does not exist; returns whether it did.
 */
function focusMatchingLegacyPanel(
  api: DockviewApi,
  keyedPanelId: string,
  legacyPanelId: string,
  paramName: string,
  key: string,
): boolean {
  if (api.getPanel(keyedPanelId)) return false;
  const legacy = api.getPanel(legacyPanelId);
  const legacyKey = (legacy?.params as Record<string, unknown> | undefined)?.[paramName];
  if (!legacy || legacyKey !== key) return false;
  legacy.api.setActive();
  return true;
}

// ---------------------------------------------------------------------------
// Preview-tab machinery
// ---------------------------------------------------------------------------

/** Preview types that support single-tab (VSCode-style) behavior. */
export type PreviewType = "file-editor" | "file-diff" | "commit-detail";

type PreviewSpec = {
  /** Stable id for the preview panel (only one per type). */
  previewId: string;
  /** Dockview `component` key used for rendering. */
  component: string;
  /** Tab component used for preview tabs (italic title, double-click to pin). */
  previewTabComponent: string;
  /** Compute the per-item pinned panel id. */
  pinnedId: (itemId: string) => string;
};

const PREVIEW_SPECS: Record<PreviewType, PreviewSpec> = {
  "file-editor": {
    previewId: "preview:file-editor",
    component: "file-editor",
    previewTabComponent: "previewFileTab",
    pinnedId: (path) => `file:${path}`,
  },
  "file-diff": {
    previewId: "preview:file-diff",
    component: "diff-viewer",
    previewTabComponent: "previewDiffTab",
    pinnedId: (path) => `diff:file:${path}`,
  },
  "commit-detail": {
    previewId: "preview:commit-detail",
    component: "commit-detail",
    previewTabComponent: "previewCommitTab",
    pinnedId: (sha) => `commit:${sha}`,
  },
};

/** Return the last path segment of `path` (the file name). */
function getFileName(path: string): string {
  return path.split("/").pop() || path;
}

/** Build a repo-scoped item id (`repo:path`) so multi-repo files get distinct panels. */
export function buildRepoScopedItemId(path: string, repo?: string): string {
  return repo ? `${repo}:${path}` : path;
}

type OpenPreviewArgs = {
  api: DockviewApi;
  type: PreviewType;
  /** Stable identifier for the item (path / sha). Used to compute pinnedId and detect no-op. */
  itemId: string;
  /** Title rendered on the tab. */
  title: string;
  /** Params to pass to the panel component (path, sha, kind, etc.). */
  params: Record<string, unknown>;
  /** Group to place the preview in when it is first created. */
  groupId: string;
  /** `quiet: true` keeps the currently active panel focused. */
  quiet?: boolean;
  /** `pin: true` forces the per-item pinned id instead of the preview slot. */
  pin?: boolean;
  /** Custom tab component for pinned opens (falls back to default dockview tab). */
  pinnedTabComponent?: string;
};

/** Move the preview panel into the explicitly requested group when it currently
 *  lives elsewhere — e.g. a saved env layout restored `preview:file-diff` into
 *  the right column, but the user just clicked a file in the changes panel
 *  expecting the diff to land in the center group. */
function movePreviewToRequestedGroup(
  preview: ReturnType<DockviewApi["getPanel"]> & object,
  api: DockviewApi,
  groupId: string,
): void {
  if (!groupId || preview.group?.id === groupId) return;
  const target = api.getGroup(groupId);
  if (!target) return;
  // `api.getGroup` returns `IDockviewGroupPanel` but `moveTo` requires the
  // concrete `DockviewGroupPanel`; at runtime they're the same object.
  preview.api.moveTo({ group: target as DockviewGroupPanel });
}

/** Update an existing preview panel with new content, materializing promoted items first. */
function updateExistingPreview(
  preview: ReturnType<DockviewApi["getPanel"]> & object,
  args: OpenPreviewArgs,
): void {
  const { api, type, itemId, title, params, groupId, quiet, pinnedTabComponent } = args;
  const currentItemId = preview.params?.previewItemId as string | undefined;
  if (preview.params?.promoted && currentItemId && currentItemId !== itemId) {
    materializePromotedPreview(api, type, pinnedTabComponent ?? PINNED_TAB);
  }
  // Preserve the promoted flag when re-opening the same item; clear it when
  // switching to a different item (the old promoted file was already
  // materialized above).
  const keepPromoted = currentItemId === itemId && !!preview.params?.promoted;
  preview.api.updateParameters({
    ...params,
    previewItemId: itemId,
    promoted: keepPromoted || undefined,
  });
  preview.setTitle(title);
  movePreviewToRequestedGroup(preview, api, groupId);
  if (!quiet) preview.api.setActive();
}

/**
 * Open the single "preview" panel for a given content type, VSCode-style.
 *
 * Lookup rules:
 *   1. If a pinned panel for the item already exists → focus it.
 *   2. Else if a preview panel for the type exists and already shows the
 *      item → focus it.
 *   3. Else if a preview panel for the type exists → replace its content
 *      (title + params) and focus it.
 *   4. Else → create a new preview panel.
 */
function openOrReplacePreview(args: OpenPreviewArgs): void {
  const { api, type, itemId, title, params, groupId, quiet, pin, pinnedTabComponent } = args;
  const spec = PREVIEW_SPECS[type];
  const pinnedId = spec.pinnedId(itemId);

  // Always prefer an existing pinned panel for this item — never disturb it.
  const pinned = api.getPanel(pinnedId);
  if (pinned) {
    if (!quiet) pinned.api.setActive();
    return;
  }

  if (pin) {
    focusOrAddPanel(
      api,
      {
        id: pinnedId,
        component: spec.component,
        title,
        params,
        ...(pinnedTabComponent ? { tabComponent: pinnedTabComponent } : {}),
        position: { referenceGroup: groupId },
      },
      quiet,
    );
    return;
  }

  const preview = api.getPanel(spec.previewId);
  if (preview) {
    updateExistingPreview(preview, args);
    return;
  }

  focusOrAddPanel(
    api,
    {
      id: spec.previewId,
      component: spec.component,
      title,
      tabComponent: spec.previewTabComponent,
      params: { ...params, previewItemId: itemId },
      position: { referenceGroup: groupId },
    },
    quiet,
  );
}

/**
 * Mark the current preview panel as "promoted" (VSCode-style keep-open).
 *
 * This does NOT swap panels — it sets a `promoted` flag on the preview's
 * params so the tab renders as non-italic (pinned look) while the editor
 * stays mounted (no remount, no focus loss).
 *
 * The actual panel swap (materialization) happens lazily when the user opens
 * a different file via {@link openOrReplacePreview}.
 */
export function promotePreviewToPinned(api: DockviewApi, type: PreviewType): void {
  const spec = PREVIEW_SPECS[type];
  const preview = api.getPanel(spec.previewId);
  if (!preview || preview.params?.promoted) return;
  preview.api.updateParameters({ ...(preview.params ?? {}), promoted: true });
}

/**
 * Materialize a promoted preview into a proper pinned panel.
 *
 * Called internally by {@link openOrReplacePreview} when the preview slot is
 * needed for a new item and the current preview was promoted.  Also available
 * for explicit "pin now" actions where an immediate panel swap is acceptable
 * (e.g. the user is navigating away anyway).
 */
function materializePromotedPreview(
  api: DockviewApi,
  type: PreviewType,
  pinnedTabComponent?: string,
): void {
  const spec = PREVIEW_SPECS[type];
  const preview = api.getPanel(spec.previewId);
  if (!preview) return;

  const itemId = preview.params?.previewItemId as string | undefined;
  if (!itemId) return;

  const pinnedId = spec.pinnedId(itemId);
  if (api.getPanel(pinnedId)) return; // Already materialized

  const groupId = preview.group?.id;
  const title = preview.title;
  const params = { ...(preview.params ?? {}) } as Record<string, unknown>;
  delete params.previewItemId;
  delete params.promoted;

  focusOrAddPanel(
    api,
    {
      id: pinnedId,
      component: spec.component,
      title,
      params,
      ...(pinnedTabComponent ? { tabComponent: pinnedTabComponent } : {}),
      ...(groupId ? { position: { referenceGroup: groupId } } : {}),
    },
    true, // quiet — don't steal focus
  );
}

export type OpenPanelOpts = {
  /** Don't steal focus from the active panel. */
  quiet?: boolean;
  /** Force the per-item pinned panel instead of the shared preview slot. */
  pin?: boolean;
  /** Multi-repo subpath (repository_name) recorded on the panel params so
   *  FileEditorPanel's fallback content fetch resolves under the right repo. */
  repo?: string;
};

export const PREVIEW_FILE_EDITOR_ID = "preview:file-editor";
const PINNED_TAB = "pinnedDefaultTab";

/**
 * Build the `addFileEditorPanel` action: opens the file-editor preview (or a
 * pinned per-item panel) for the given path, keyed repo-scoped when a repo
 * is provided.
 */
function buildFileEditorAction(get: StoreGet) {
  return (path: string, name: string, opts?: OpenPanelOpts) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    const itemId = buildRepoScopedItemId(path, opts?.repo);
    openOrReplacePreview({
      api,
      type: "file-editor",
      itemId,
      title: name,
      params: { path, ...(opts?.repo ? { repo: opts.repo } : {}) },
      groupId: centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

function buildFileDiffItemId(path: string, scope: string | undefined, layer?: ChangeLayer): string {
  const scopedItemId = buildRepoScopedItemId(path, scope);
  return layer ? `${layer}:${scopedItemId}` : scopedItemId;
}

/**
 * Build the `addFileDiffPanel` action: opens the file-diff preview (or pinned
 * panel) for a path, passing content/source/repository/pr params through and
 * keying the item repo-scoped.
 */
function buildFileDiffAction(get: StoreGet) {
  return (
    path: string,
    opts?: OpenPanelOpts & {
      content?: string;
      groupId?: string;
      source?: string;
      repositoryName?: string;
      prKey?: string;
      changeLayer?: ChangeLayer;
    },
  ) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    const itemId = buildFileDiffItemId(
      path,
      opts?.prKey ?? opts?.repositoryName,
      opts?.changeLayer,
    );
    openOrReplacePreview({
      api,
      type: "file-diff",
      itemId,
      title: t("task:panelFileDiff", { file: getFileName(path) }),
      params: {
        kind: "file",
        path,
        content: opts?.content,
        source: opts?.source,
        repositoryName: opts?.repositoryName,
        prKey: opts?.prKey,
        changeLayer: opts?.changeLayer,
      },
      groupId: opts?.groupId ?? centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

/**
 * Compute the stable panel id for a commit detail target — legacy string
 * targets keep the raw (or `legacyRepo`-prefixed) id, while discriminated
 * targets encode provenance (`local:`/`github:`) and repository identity.
 */
function buildCommitItemId(
  requestedTarget: CommitDetailTarget | string,
  target: CommitDetailTarget,
  legacyRepo?: string,
): string {
  if (typeof requestedTarget === "string") {
    return legacyRepo ? `${legacyRepo}:${requestedTarget}` : requestedTarget;
  }
  if (target.source === "local") return `local:${target.repo ?? ""}:${target.sha}`;
  return `github:${target.workspaceId}:${target.owner}/${target.repo}:${target.sha}`;
}

/**
 * Build the `addCommitDetailPanel` action: opens the commit-detail preview
 * for a legacy sha string or a discriminated commit target.
 */
function buildCommitDetailAction(get: StoreGet) {
  return (
    requestedTarget: CommitDetailTarget | string,
    opts?: OpenPanelOpts & { groupId?: string; repo?: string },
  ) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    const target: CommitDetailTarget =
      typeof requestedTarget === "string"
        ? {
            source: "local",
            sha: requestedTarget,
            ...(opts?.repo ? { repo: opts.repo } : {}),
          }
        : requestedTarget;
    // Preserve the legacy string-call identity for saved callers/tests; new
    // discriminated targets include provenance and repository identity.
    const itemId = buildCommitItemId(requestedTarget, target, opts?.repo);
    openOrReplacePreview({
      api,
      type: "commit-detail",
      itemId,
      title: target.sha.slice(0, 7),
      params: {
        target,
        commitSha: target.sha,
        ...(target.source === "local" && target.repo ? { repo: target.repo } : {}),
      },
      groupId: opts?.groupId ?? centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

/**
 * Build the store's core panel actions: chat, changes/files, diff viewer,
 * file diff/editor, commit detail, browser, and preview pinning.
 */
export function buildPanelActions(set: StoreSet, get: StoreGet) {
  return {
    addChatPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "chat",
        component: "chat",
        tabComponent: "permanentTab",
        title: panelTitle("chat"),
        position: { referenceGroup: centerGroupId },
      });
    },
    addChangesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "changes",
        component: "changes",
        title: panelTitle("changes"),
        tabComponent: "changesTab",
      });
    },
    addFilesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "files",
        component: "files",
        title: panelTitle("files"),
      });
    },
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      if (path) set({ selectedDiff: { path, content } });
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: "diff-viewer",
        component: "diff-viewer",
        title: t("task:panelDiffViewer"),
        params: { kind: "all" },
      });
    },
    addFileDiffPanel: buildFileDiffAction(get),
    addCommitDetailPanel: buildCommitDetailAction(get),
    addFileEditorPanel: buildFileEditorAction(get),
    addBrowserPanel: (url?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const browserId = url ? `browser:${url}` : `browser:${Date.now()}`;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: browserId,
        component: "browser",
        title: panelTitle("browser"),
        params: { url: url ?? "" },
      });
    },
    /**
     * Open the preview browser without stacking a tab per call.
     *
     * `addBrowserPanel()` mints `browser:<timestamp>` when it has no URL, so an
     * automatic open (start dev server, then stop, then start again) left a new
     * empty tab behind on every cycle, and a URL detected later updated only the
     * first of them. Explicit "+ > Browser" still uses `addBrowserPanel`, where
     * a second tab is what the user asked for.
     */
    focusOrAddBrowserPanel: (groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const existing = findBrowserPanel(api);
      if (existing) {
        existing.api.setActive();
        return;
      }
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: `browser:${Date.now()}`,
        component: "browser",
        title: panelTitle("browser"),
        params: { url: "" },
      });
    },
    openBrowserPanel: (url: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      openBrowserPanel(api, centerGroupId, url);
    },
    promotePreviewToPinned: (type: PreviewType): void => {
      const { api } = get();
      if (!api) return;
      promotePreviewToPinned(api, type);
    },
  };
}

/** Add a session tab to the center group. */
export function addSessionPanel(
  api: DockviewApi,
  centerGroupId: string,
  sessionId: string,
  title: string,
): void {
  focusOrAddPanel(api, {
    id: `session:${sessionId}`,
    component: "chat",
    tabComponent: "sessionTab",
    title,
    params: { sessionId },
    position: { referenceGroup: centerGroupId },
  });
}

/** Remove a session tab panel if it exists. */
export function removeSessionPanel(api: DockviewApi, sessionId: string): void {
  const panel = api.getPanel(`session:${sessionId}`);
  if (panel) api.removePanel(panel);
}

/**
 * Build the review-detail panel actions (`addPRPanel`, `addMRPanel`,
 * `addReviewPanel`), each focusing an existing matching panel (canonical or
 * keyed) or adding a keyed detail panel in the requested group.
 */
export function buildReviewPanelActions(get: StoreGet) {
  return {
    /**
     * Focus an existing PR tab in place, or add a keyed tab in the explicitly
     * requested group. Callers without a group use the layout-owned canonical
     * PR Details panel, then the center fallback.
     */
    addPRPanel: (prKey?: string, opts?: ReviewPanelOptions) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      // Multi-repo: each TaskPR opens in its own panel keyed by
      // owner/repo/pr_number so multiple PRs can be tabbed side-by-side.
      // Legacy single-repo callers (no key) get the historical panel id.
      const id = prKey ? `pr-detail|${prKey}` : "pr-detail";
      // A canonical panel already rendering this key is the task's primary
      // review tab; focus it instead of creating a duplicate keyed tab.
      const existing = api.getPanel(id);
      if (existing) {
        existing.api.setActive();
        return;
      }
      if (prKey && focusMatchingLegacyPanel(api, id, "pr-detail", "prKey", prKey)) return;
      const targetGroupId = opts?.groupId ?? api.getPanel("pr-detail")?.group.id ?? centerGroupId;
      focusOrAddPanel(api, {
        id,
        component: "pr-detail",
        title: prKey ? t("task:panelPullRequest") : panelTitle("pr-detail"),
        position: { referenceGroup: targetGroupId },
        params: prKey ? { prKey } : undefined,
      });
    },
    addMRPanel: (mrKey: string, opts?: ReviewPanelOptions) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const id = `mr-detail|${mrKey}`;
      const existing = api.getPanel(id);
      if (existing) {
        existing.api.setActive();
        return;
      }
      if (focusMatchingLegacyPanel(api, id, "mr-detail", "mrKey", mrKey)) return;
      const canonical = api.getPanel("pr-detail");
      if (canonical?.params?.mrKey === mrKey) {
        canonical.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id,
        component: "mr-detail",
        title: panelTitle("mr-detail"),
        position: { referenceGroup: opts?.groupId ?? canonical?.group.id ?? centerGroupId },
        params: { mrKey },
      });
    },
    addReviewPanel: (review: ReviewPanelTarget, opts?: ReviewPanelOptions) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const canonical = api.getPanel("pr-detail");
      if (
        canonical?.params?.providerId === review.providerId &&
        canonical.params.connectionScope === review.connectionScope &&
        canonical.params.repositoryId === review.repositoryId &&
        String(canonical.params.changeRequestNumber) === String(review.changeRequestNumber)
      ) {
        canonical.api.setActive();
        return;
      }
      const id = reviewPanelId(review);
      const existing = api.getPanel(id);
      if (existing) {
        existing.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id,
        component: "review-detail",
        title: review.title,
        position: { referenceGroup: opts?.groupId ?? canonical?.group.id ?? centerGroupId },
        params: {
          providerId: review.providerId,
          reviewKey: review.reviewKey,
          connectionScope: review.connectionScope,
          repositoryId: review.repositoryId,
          changeRequestNumber: review.changeRequestNumber,
        },
      });
    },
  };
}
