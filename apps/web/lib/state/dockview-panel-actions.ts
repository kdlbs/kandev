import type { DockviewApi, AddPanelOptions } from "dockview-react";
import { focusOrAddPanel } from "./dockview-layout-builders";

type StoreGet = () => {
  api: DockviewApi | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  selectedDiff: { path: string; content?: string } | null;
};
type StoreSet = (
  partial: Partial<{ selectedDiff: { path: string; content?: string } | null }>,
) => void;

type SimplePanelOpts = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
};

function addSimplePanel(api: DockviewApi, groupId: string, opts: SimplePanelOpts): void {
  focusOrAddPanel(api, { ...opts, position: { referenceGroup: groupId } });
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

function getFileName(path: string): string {
  return path.split("/").pop() || path;
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
    const sameItem = preview.params?.previewItemId === itemId;
    if (!sameItem) {
      preview.api.updateParameters({ ...params, previewItemId: itemId });
      preview.setTitle(title);
    }
    if (!quiet) preview.api.setActive();
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
 * Promote the current preview panel for a type into a pinned (per-item) panel.
 * Returns the new pinned panel id, or null when there is no preview to promote.
 */
export function promotePreviewToPinned(
  api: DockviewApi,
  type: PreviewType,
  pinnedTabComponent?: string,
): string | null {
  const spec = PREVIEW_SPECS[type];
  const preview = api.getPanel(spec.previewId);
  if (!preview) return null;

  const itemId = preview.params?.previewItemId as string | undefined;
  if (!itemId) return null;

  const pinnedId = spec.pinnedId(itemId);
  if (api.getPanel(pinnedId)) {
    // Pinned already exists for this item (unlikely). Just drop the preview.
    api.removePanel(preview);
    return pinnedId;
  }

  const groupId = preview.group?.id;
  const title = preview.title;
  const { previewItemId: _drop, ...params } = { ...(preview.params ?? {}) };
  void _drop;

  api.removePanel(preview);
  focusOrAddPanel(api, {
    id: pinnedId,
    component: spec.component,
    title,
    params,
    ...(pinnedTabComponent ? { tabComponent: pinnedTabComponent } : {}),
    ...(groupId ? { position: { referenceGroup: groupId } } : {}),
  });
  return pinnedId;
}

export type OpenPanelOpts = {
  /** Don't steal focus from the active panel. */
  quiet?: boolean;
  /** Force the per-item pinned panel instead of the shared preview slot. */
  pin?: boolean;
};

const PINNED_TAB = "pinnedDefaultTab";

function buildFileEditorAction(get: StoreGet) {
  return (path: string, name: string, opts?: OpenPanelOpts) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    openOrReplacePreview({
      api,
      type: "file-editor",
      itemId: path,
      title: name,
      params: { path },
      groupId: centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

function buildFileDiffAction(get: StoreGet) {
  return (path: string, opts?: OpenPanelOpts & { content?: string; groupId?: string }) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    openOrReplacePreview({
      api,
      type: "file-diff",
      itemId: path,
      title: `Diff [${getFileName(path)}]`,
      params: { kind: "file", path, content: opts?.content },
      groupId: opts?.groupId ?? centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

function buildCommitDetailAction(get: StoreGet) {
  return (sha: string, opts?: OpenPanelOpts & { groupId?: string }) => {
    const { api, centerGroupId } = get();
    if (!api) return;
    openOrReplacePreview({
      api,
      type: "commit-detail",
      itemId: sha,
      title: sha.slice(0, 7),
      params: { commitSha: sha },
      groupId: opts?.groupId ?? centerGroupId,
      quiet: opts?.quiet,
      pin: opts?.pin,
      pinnedTabComponent: PINNED_TAB,
    });
  };
}

export function buildPanelActions(set: StoreSet, get: StoreGet) {
  return {
    addChatPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "chat",
        component: "chat",
        tabComponent: "permanentTab",
        title: "Agent",
        position: { referenceGroup: centerGroupId },
      });
    },
    addChangesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "changes",
        component: "changes",
        title: "Changes",
        tabComponent: "changesTab",
      });
    },
    addFilesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "files",
        component: "files",
        title: "Files",
      });
    },
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      if (path) set({ selectedDiff: { path, content } });
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: "diff-viewer",
        component: "diff-viewer",
        title: "Diff Viewer",
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
        title: "Browser",
        params: { url: url ?? "" },
      });
    },
    promotePreviewToPinned: (type: PreviewType): string | null => {
      const { api } = get();
      if (!api) return null;
      return promotePreviewToPinned(api, type, PINNED_TAB);
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

export function buildExtraPanelActions(get: StoreGet) {
  return {
    addVscodePanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referenceGroup: centerGroupId },
      });
    },
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    openInternalVscode: (_goto: { file: string; line: number; col: number } | null) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const existing = api.getPanel("vscode");
      if (existing) {
        existing.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referenceGroup: centerGroupId },
      });
    },
    addPlanPanel: (groupId?: string) => {
      const { api } = get();
      if (!api) return;
      const position = groupId
        ? { referenceGroup: groupId }
        : { referencePanel: "chat" as const, direction: "right" as const };
      focusOrAddPanel(api, { id: "plan", component: "plan", title: "Plan", position });
    },
    addPRPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "pr-detail",
        component: "pr-detail",
        title: "Pull Request",
        position: { referenceGroup: centerGroupId },
      });
    },
    addTerminalPanel: (terminalId?: string, groupId?: string) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      const id = terminalId ?? `terminal-${Date.now()}`;
      addSimplePanel(api, groupId ?? rightBottomGroupId, {
        id,
        component: "terminal",
        title: "Terminal",
        params: { terminalId: id },
      });
    },
  };
}

// Keep a bogus reference so AddPanelOptions import isn't elided by ts-unused.
type _Keep = AddPanelOptions;
void (null as unknown as _Keep);
