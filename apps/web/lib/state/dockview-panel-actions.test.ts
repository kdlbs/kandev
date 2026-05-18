import { describe, it, expect, beforeEach } from "vitest";
import type { DockviewApi, AddPanelOptions } from "dockview-react";
import { buildPanelActions, buildExtraPanelActions } from "./dockview-panel-actions";
import { CENTER_GROUP } from "./layout-manager";

// ---------------------------------------------------------------------------
// Minimal DockviewApi mock
// ---------------------------------------------------------------------------

type MockPanel = {
  id: string;
  title: string;
  params: Record<string, unknown>;
  group: { id: string };
  isActive: boolean;
  api: {
    setActive: () => void;
    updateParameters: (p: Record<string, unknown>) => void;
  };
  setTitle: (t: string) => void;
};

function makeApi(options: { centerGroupId?: string } = {}): DockviewApi {
  const centerId = options.centerGroupId ?? CENTER_GROUP;
  const panels: MockPanel[] = [];
  const groups = [{ id: centerId }];

  function makePanel(add: AddPanelOptions & { id: string }): MockPanel {
    const groupId =
      (add.position as { referenceGroup?: string } | undefined)?.referenceGroup ?? centerId;
    if (!groups.some((g) => g.id === groupId)) groups.push({ id: groupId });
    const panel: MockPanel = {
      id: add.id,
      title: (add.title as string) ?? "",
      params: { ...(add.params ?? {}) },
      group: { id: groupId },
      isActive: false,
      setTitle(t: string) {
        this.title = t;
      },
      api: {
        setActive() {
          for (const p of panels) p.isActive = false;
          panel.isActive = true;
        },
        updateParameters(p: Record<string, unknown>) {
          Object.assign(panel.params, p);
        },
      },
    };
    return panel;
  }

  const api = {
    get groups() {
      return groups;
    },
    get panels() {
      return panels;
    },
    getPanel(id: string) {
      return panels.find((p) => p.id === id);
    },
    addPanel(opts: AddPanelOptions & { id: string }) {
      const p = makePanel(opts);
      panels.push(p);
      if (!opts.inactive) p.api.setActive();
      return p;
    },
    removePanel(panel: { id: string }) {
      const i = panels.findIndex((p) => p.id === panel.id);
      if (i >= 0) panels.splice(i, 1);
    },
    get activePanel() {
      return panels.find((p) => p.isActive);
    },
  } as unknown as DockviewApi;
  return api;
}

// ---------------------------------------------------------------------------
// Store adapter: panel actions take (set, get) closures
// ---------------------------------------------------------------------------

type StoreShape = {
  api: DockviewApi | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  selectedDiff: { path: string; content?: string } | null;
};

function makeStore(api: DockviewApi) {
  const state: StoreShape = {
    api,
    centerGroupId: CENTER_GROUP,
    rightTopGroupId: "group-right-top",
    rightBottomGroupId: "group-right-bottom",
    selectedDiff: null,
  };
  return {
    get: () => state,
    set: (partial: Partial<StoreShape>) => Object.assign(state, partial),
    state,
  };
}

function build(api: DockviewApi) {
  const store = makeStore(api);
  const actions = buildPanelActions(store.set, store.get);
  return { api, actions, store };
}

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

const PATH_A = "src/a.ts";
const PATH_B = "src/b.ts";
const PATH_NESTED_B = "src/nested/b.ts";
const NAME_A = "a.ts";
const NAME_B = "b.ts";
const PINNED_FILE_A_ID = "file:src/a.ts";
const PREVIEW_FILE_ID = "preview:file-editor";
const PREVIEW_DIFF_ID = "preview:file-diff";
const PREVIEW_COMMIT_ID = "preview:commit-detail";
const SHA_A = "abcdef1234567890";
const SHA_B = "fedcba0987654321";
const DIFF_FILE_PREFIX = "diff:file:";
const FILE_PREFIX_ID = "file:";
const COMMIT_PREFIX_ID = "commit:";
const TYPE_FILE_EDITOR = "file-editor" as const;

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("addFileEditorPanel — preview behavior", () => {
  let api: DockviewApi;
  let actions: ReturnType<typeof buildPanelActions>;
  beforeEach(() => {
    ({ api, actions } = build(makeApi()));
  });

  it("first open creates a preview panel with the stable preview id", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);

    expect(api.getPanel(PREVIEW_FILE_ID)).toBeDefined();
    expect(api.getPanel(PINNED_FILE_A_ID)).toBeUndefined();
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview.title).toBe(NAME_A);
    expect(preview.params.path).toBe(PATH_A);
  });

  it("second open (different file) replaces the preview in place", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.addFileEditorPanel(PATH_B, NAME_B);

    // Still exactly one file panel (preview), now showing b
    const filePanels = api.panels.filter(
      (p) => p.id === PREVIEW_FILE_ID || p.id.startsWith(FILE_PREFIX_ID),
    );
    expect(filePanels).toHaveLength(1);

    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview.title).toBe(NAME_B);
    expect(preview.params.path).toBe(PATH_B);
  });

  it("focuses the pinned panel instead of touching preview when pinned exists", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A, { pin: true });
    actions.addFileEditorPanel(PATH_B, NAME_B); // preview for b
    expect(api.getPanel(PINNED_FILE_A_ID)).toBeDefined();
    expect(api.getPanel(PREVIEW_FILE_ID)).toBeDefined();

    // Re-open a: should activate the pinned panel and leave preview alone
    actions.addFileEditorPanel(PATH_A, NAME_A);

    const pinned = api.getPanel(PINNED_FILE_A_ID) as unknown as MockPanel;
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(pinned.isActive).toBe(true);
    expect(preview.title).toBe(NAME_B); // unchanged
    expect(preview.params.path).toBe(PATH_B); // unchanged
  });

  it("opens directly pinned when pin option is true", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A, { pin: true });

    expect(api.getPanel(PINNED_FILE_A_ID)).toBeDefined();
    expect(api.getPanel(PREVIEW_FILE_ID)).toBeUndefined();
  });

  it("promotePreviewToPinned sets promoted flag without swapping panels", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);

    // Preview panel still exists with promoted flag
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.promoted).toBe(true);
    expect(preview.title).toBe(NAME_A);
    // No pinned panel created yet
    expect(api.getPanel(PINNED_FILE_A_ID)).toBeUndefined();
  });

  it("opening a new file materializes the promoted preview as a pinned panel", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);
    actions.addFileEditorPanel(PATH_B, NAME_B);

    // Promoted file A was materialized as pinned
    expect(api.getPanel(PINNED_FILE_A_ID)).toBeDefined();
    const pinned = api.getPanel(PINNED_FILE_A_ID) as unknown as MockPanel;
    expect(pinned.params.path).toBe(PATH_A);
    expect(pinned.params.promoted).toBeUndefined();
    expect(pinned.params.previewItemId).toBeUndefined();
    // Preview now shows file B
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview.params.path).toBe(PATH_B);
    expect(preview.params.promoted).toBeUndefined();
  });

  it("re-opening the same promoted preview just focuses without materializing", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);
    actions.addFileEditorPanel(PATH_A, NAME_A);

    // No pinned panel created — same item, no materialization
    expect(api.getPanel(PINNED_FILE_A_ID)).toBeUndefined();
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview.params.promoted).toBe(true);
    expect(preview.isActive).toBe(true);
  });

  it("promotePreviewToPinned is a no-op when no preview exists", () => {
    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);
    expect(api.panels).toHaveLength(0);
  });

  it("promotePreviewToPinned is a no-op when already promoted", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    const paramsRef = preview.params;

    actions.promotePreviewToPinned(TYPE_FILE_EDITOR);
    // params reference unchanged (no second updateParameters call)
    expect(preview.params).toBe(paramsRef);
  });

  it("re-opening the same preview target just focuses without churning params", () => {
    actions.addFileEditorPanel(PATH_A, NAME_A);
    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    const originalParamsRef = preview.params;

    actions.addFileEditorPanel(PATH_A, NAME_A);

    // Same preview panel — updateParameters mutates params in-place (Object.assign),
    // so the reference is unchanged even after the call.
    expect(api.getPanel(PREVIEW_FILE_ID)).toBe(preview as unknown);
    expect(preview.params).toBe(originalParamsRef);
    expect(preview.isActive).toBe(true);
  });
});

describe("addFileDiffPanel — preview behavior", () => {
  let api: DockviewApi;
  let actions: ReturnType<typeof buildPanelActions>;
  beforeEach(() => {
    ({ api, actions } = build(makeApi()));
  });

  it("first open creates a preview diff panel with the stable preview id", () => {
    actions.addFileDiffPanel(PATH_A);

    expect(api.getPanel(PREVIEW_DIFF_ID)).toBeDefined();
    expect(api.getPanel(`${DIFF_FILE_PREFIX}${PATH_A}`)).toBeUndefined();
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview.title).toBe("Diff [a.ts]");
    expect(preview.params.path).toBe(PATH_A);
    expect(preview.params.kind).toBe("file");
  });

  it("second diff open replaces the preview in place", () => {
    actions.addFileDiffPanel(PATH_A);
    actions.addFileDiffPanel(PATH_NESTED_B);

    const diffPanels = api.panels.filter(
      (p) => p.id === PREVIEW_DIFF_ID || p.id.startsWith(DIFF_FILE_PREFIX),
    );
    expect(diffPanels).toHaveLength(1);
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview.title).toBe("Diff [b.ts]");
    expect(preview.params.path).toBe(PATH_NESTED_B);
  });

  it("focuses pinned diff when present instead of touching preview", () => {
    actions.addFileDiffPanel(PATH_A, { pin: true });
    actions.addFileDiffPanel(PATH_B); // preview
    actions.addFileDiffPanel(PATH_A);

    const pinned = api.getPanel(`${DIFF_FILE_PREFIX}${PATH_A}`) as unknown as MockPanel;
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(pinned.isActive).toBe(true);
    expect(preview.params.path).toBe(PATH_B);
  });

  it("promotePreviewToPinned sets promoted flag on the preview diff", () => {
    actions.addFileDiffPanel(PATH_A);
    actions.promotePreviewToPinned("file-diff");

    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.promoted).toBe(true);
    expect(api.getPanel(`${DIFF_FILE_PREFIX}${PATH_A}`)).toBeUndefined();
  });

  it("opening a new diff materializes the promoted diff as a pinned panel", () => {
    actions.addFileDiffPanel(PATH_A);
    actions.promotePreviewToPinned("file-diff");
    actions.addFileDiffPanel(PATH_B);

    expect(api.getPanel(`${DIFF_FILE_PREFIX}${PATH_A}`)).toBeDefined();
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview.params.path).toBe(PATH_B);
    expect(preview.params.promoted).toBeUndefined();
  });
});

describe("addCommitDetailPanel — preview behavior", () => {
  let api: DockviewApi;
  let actions: ReturnType<typeof buildPanelActions>;
  beforeEach(() => {
    ({ api, actions } = build(makeApi()));
  });

  it("first open creates a preview commit panel with the stable preview id", () => {
    actions.addCommitDetailPanel(SHA_A);

    expect(api.getPanel(PREVIEW_COMMIT_ID)).toBeDefined();
    expect(api.getPanel(`${COMMIT_PREFIX_ID}${SHA_A}`)).toBeUndefined();
    const preview = api.getPanel(PREVIEW_COMMIT_ID) as unknown as MockPanel;
    expect(preview.title).toBe("abcdef1");
    expect(preview.params.commitSha).toBe(SHA_A);
  });

  it("second commit open replaces the preview in place", () => {
    actions.addCommitDetailPanel(SHA_A);
    actions.addCommitDetailPanel(SHA_B);

    const commitPanels = api.panels.filter(
      (p) => p.id === PREVIEW_COMMIT_ID || p.id.startsWith(COMMIT_PREFIX_ID),
    );
    expect(commitPanels).toHaveLength(1);
    const preview = api.getPanel(PREVIEW_COMMIT_ID) as unknown as MockPanel;
    expect(preview.title).toBe("fedcba0");
    expect(preview.params.commitSha).toBe(SHA_B);
  });

  it("promotePreviewToPinned sets promoted flag on the preview commit", () => {
    actions.addCommitDetailPanel(SHA_A);
    actions.promotePreviewToPinned("commit-detail");

    const preview = api.getPanel(PREVIEW_COMMIT_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.promoted).toBe(true);
    expect(api.getPanel(`${COMMIT_PREFIX_ID}${SHA_A}`)).toBeUndefined();
  });

  it("opening a new commit materializes the promoted commit as a pinned panel", () => {
    actions.addCommitDetailPanel(SHA_A);
    actions.promotePreviewToPinned("commit-detail");
    actions.addCommitDetailPanel(SHA_B);

    expect(api.getPanel(`${COMMIT_PREFIX_ID}${SHA_A}`)).toBeDefined();
    const preview = api.getPanel(PREVIEW_COMMIT_ID) as unknown as MockPanel;
    expect(preview.params.commitSha).toBe(SHA_B);
    expect(preview.params.promoted).toBeUndefined();
  });
});

describe("preview slots are independent across types", () => {
  it("file, diff, and commit previews coexist", () => {
    const { api, actions } = build(makeApi());

    actions.addFileEditorPanel(PATH_A, NAME_A);
    actions.addFileDiffPanel(PATH_B);
    actions.addCommitDetailPanel(SHA_A);

    expect(api.getPanel(PREVIEW_FILE_ID)).toBeDefined();
    expect(api.getPanel(PREVIEW_DIFF_ID)).toBeDefined();
    expect(api.getPanel(PREVIEW_COMMIT_ID)).toBeDefined();
  });
});

describe("addPRPanel — dedup with legacy auto-shown panel", () => {
  const PR_KEY = "testorg/testrepo/101";
  const LEGACY_PR_ID = "pr-detail";
  const KEYED_PR_ID = `pr-detail|${PR_KEY}`;

  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.get) };
  }

  it("focuses the legacy auto-shown panel instead of creating a keyed duplicate", () => {
    const { api, actions } = buildExtra(makeApi());

    // Auto-show creates the legacy (unkeyed) panel.
    actions.addPRPanel();
    expect(api.getPanel(LEGACY_PR_ID)).toBeDefined();

    // Topbar click with a prKey must reuse the legacy panel, not add a second tab.
    actions.addPRPanel(PR_KEY);

    const prPanels = api.panels.filter(
      (p) => p.id === LEGACY_PR_ID || p.id.startsWith(`${LEGACY_PR_ID}|`),
    );
    expect(prPanels).toHaveLength(1);
    expect(api.getPanel(KEYED_PR_ID)).toBeUndefined();

    // Legacy panel was updated with the prKey so it renders the requested PR.
    const legacy = api.getPanel(LEGACY_PR_ID) as unknown as MockPanel;
    expect(legacy.params.prKey).toBe(PR_KEY);
    expect(legacy.isActive).toBe(true);
  });

  it("creates a new keyed panel when no legacy panel exists", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPRPanel(PR_KEY);

    expect(api.getPanel(KEYED_PR_ID)).toBeDefined();
    expect(api.getPanel(LEGACY_PR_ID)).toBeUndefined();
  });

  it("focuses an existing keyed panel exactly once", () => {
    const { api, actions } = buildExtra(makeApi());

    actions.addPRPanel(PR_KEY);
    actions.addPRPanel(PR_KEY);

    const prPanels = api.panels.filter(
      (p) => p.id === LEGACY_PR_ID || p.id.startsWith(`${LEGACY_PR_ID}|`),
    );
    expect(prPanels).toHaveLength(1);
    const keyed = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(keyed.isActive).toBe(true);
  });
});

describe("addPRPanel — group placement", () => {
  const PR_KEY = "testorg/testrepo/202";
  const KEYED_PR_ID = `pr-detail|${PR_KEY}`;
  const SESSION_ID = "s-1";
  const SESSION_PANEL_ID = `session:${SESSION_ID}`;
  const SESSION_GROUP = "group-session-host";

  function buildExtra(api: DockviewApi) {
    const store = makeStore(api);
    return { api, actions: buildExtraPanelActions(store.get) };
  }

  // Seed the api with a session panel in a group that is NOT the store's
  // centerGroupId. This mirrors the post-transition state where the store's
  // tracked centerGroupId has gone stale and the live session sits elsewhere.
  function seedSessionInGroup(api: DockviewApi, groupId: string): void {
    api.addPanel({
      id: SESSION_PANEL_ID,
      component: "chat",
      title: "Session",
      params: { sessionId: SESSION_ID },
      position: { referenceGroup: groupId },
    });
  }

  it("places the PR panel in the same group as the active session panel", () => {
    const { api, actions } = buildExtra(makeApi());
    seedSessionInGroup(api, SESSION_GROUP);

    actions.addPRPanel(PR_KEY, SESSION_ID);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    const session = api.getPanel(SESSION_PANEL_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(session.group.id);
    expect(pr.group.id).toBe(SESSION_GROUP);
    // Critical: must NOT have landed in the store's centerGroupId (the bug).
    expect(pr.group.id).not.toBe(CENTER_GROUP);
  });

  it("falls back to centerGroupId when no session panel exists for the id", () => {
    const { api, actions } = buildExtra(makeApi());
    // No session panel seeded — resolver should fall back.
    actions.addPRPanel(PR_KEY, SESSION_ID);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(CENTER_GROUP);
  });

  it("falls back to centerGroupId when sessionId is not provided", () => {
    const { api, actions } = buildExtra(makeApi());
    // Even with a session panel present, omitting the id keeps legacy behavior.
    seedSessionInGroup(api, SESSION_GROUP);
    actions.addPRPanel(PR_KEY);

    const pr = api.getPanel(KEYED_PR_ID) as unknown as MockPanel;
    expect(pr).toBeDefined();
    expect(pr.group.id).toBe(CENTER_GROUP);
  });
});
