import { describe, it, expect, beforeEach } from "vitest";
import type { DockviewApi } from "dockview-react";
import { buildPanelActions } from "./dockview-panel-actions";
import { CENTER_GROUP } from "./layout-manager";
import { build, makeApi } from "./dockview-panel-actions.test-utils";
import type { MockPanel } from "./dockview-panel-actions.test-utils";
import {
  COMMIT_PREFIX_ID,
  DIFF_FILE_PREFIX,
  FILE_PREFIX_ID,
  NAME_A,
  NAME_B,
  PATH_A,
  PATH_B,
  PATH_NESTED_B,
  PINNED_FILE_A_ID,
  PREVIEW_COMMIT_ID,
  PREVIEW_DIFF_ID,
  PREVIEW_FILE_ID,
  SHA_A,
  SHA_B,
  SHARED_NAME,
  SHARED_PATH,
  TYPE_FILE_EDITOR,
} from "./dockview-panel-actions.test-utils";

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const BROWSER_COMPONENT = "browser";
const BROWSER_GROUP = "group-browser";
const EXAMPLE_URL = "https://example.test";
const NEW_URL = `${EXAMPLE_URL}/new/`;

describe("openBrowserPanel", () => {
  let api: DockviewApi;
  let actions: ReturnType<typeof buildPanelActions>;

  beforeEach(() => {
    ({ api, actions } = build(makeApi({ extraGroupIds: [BROWSER_GROUP] })));
  });

  it("creates and activates a Browser panel in the center group", () => {
    const url = `${EXAMPLE_URL}/port-proxy/session/3000/`;

    actions.openBrowserPanel(url);

    const browser = api.panels.find(
      (panel) => panel.api.component === BROWSER_COMPONENT,
    ) as unknown as MockPanel;
    expect(browser).toBeDefined();
    expect(browser.id).toBe(`browser:${url}`);
    expect(browser.group.id).toBe(CENTER_GROUP);
    expect(browser.params.url).toBe(url);
    expect(browser.isActive).toBe(true);
  });

  it("uses the Dockview placement fallback when the stored center group is stale", () => {
    const { api: staleApi, actions: staleActions, store } = build(makeApi());
    store.set({ centerGroupId: "stale-center-group" });

    staleActions.openBrowserPanel(NEW_URL);

    const browser = staleApi.panels.find((panel) => panel.api.component === BROWSER_COMPONENT);
    expect(browser?.group.id).toBe(CENTER_GROUP);
  });

  it("updates and focuses the active Browser panel without creating a duplicate", () => {
    const oldUrl = `${EXAMPLE_URL}/old/`;
    const newUrl = NEW_URL;
    api.addPanel({
      id: `browser:${oldUrl}`,
      component: BROWSER_COMPONENT,
      title: "Browser",
      params: { url: oldUrl },
      position: { referenceGroup: BROWSER_GROUP },
    });

    actions.openBrowserPanel(newUrl);

    const browsers = api.panels.filter(
      (panel) => panel.api.component === BROWSER_COMPONENT,
    ) as unknown as MockPanel[];
    expect(browsers).toHaveLength(1);
    const browser = browsers[0]!;
    expect(browser.params.url).toBe(newUrl);
    expect(browser.isActive).toBe(true);
    expect(browser.group.id).toBe(BROWSER_GROUP);
  });

  it("prefers the active Browser panel over an earlier Browser panel", () => {
    const firstUrl = `${EXAMPLE_URL}/first/`;
    const secondUrl = `${EXAMPLE_URL}/second/`;
    const newUrl = NEW_URL;
    api.addPanel({
      id: `browser:${firstUrl}`,
      component: BROWSER_COMPONENT,
      title: "Browser",
      params: { url: firstUrl },
      position: { referenceGroup: BROWSER_GROUP },
    });
    const second = api.addPanel({
      id: `browser:${secondUrl}`,
      component: BROWSER_COMPONENT,
      title: "Browser",
      params: { url: secondUrl },
      position: { referenceGroup: CENTER_GROUP },
    });
    second.api.setActive();

    actions.openBrowserPanel(newUrl);

    expect((api.getPanel(`browser:${firstUrl}`) as unknown as MockPanel).params.url).toBe(firstUrl);
    expect((api.getPanel(`browser:${secondUrl}`) as unknown as MockPanel).params.url).toBe(newUrl);
    expect((api.getPanel(`browser:${secondUrl}`) as unknown as MockPanel).isActive).toBe(true);
  });

  it("uses the first Browser panel when another panel is active", () => {
    const browserUrl = `${EXAMPLE_URL}/browser/`;
    const chat = api.addPanel({
      id: "chat",
      component: "chat",
      title: "Chat",
      position: { referenceGroup: CENTER_GROUP },
    });
    const browser = api.addPanel({
      id: `browser:${browserUrl}`,
      component: BROWSER_COMPONENT,
      title: "Browser",
      params: { url: browserUrl },
      position: { referenceGroup: BROWSER_GROUP },
    });
    chat.api.setActive();

    actions.openBrowserPanel(NEW_URL);

    const mockBrowser = browser as unknown as MockPanel;
    expect(mockBrowser.params.url).toBe(NEW_URL);
    expect(mockBrowser.isActive).toBe(true);
  });
});

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

describe("addFileEditorPanel — multi-repo identity", () => {
  it("does not reuse a pinned editor for the same path in a different repo", () => {
    const { api, actions } = build(makeApi());

    actions.addFileEditorPanel(SHARED_PATH, SHARED_NAME, { pin: true, repo: "frontend" });
    actions.addFileEditorPanel(SHARED_PATH, SHARED_NAME, { repo: "backend" });

    const preview = api.getPanel(PREVIEW_FILE_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.path).toBe(SHARED_PATH);
    expect(preview.params.repo).toBe("backend");
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

  it("does not reuse a pinned diff for the same path in a different repo", () => {
    actions.addFileDiffPanel(SHARED_PATH, { pin: true, repositoryName: "frontend" });

    actions.addFileDiffPanel(SHARED_PATH, { repositoryName: "backend" });

    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.path).toBe(SHARED_PATH);
    expect(preview.params.repositoryName).toBe("backend");
    expect(preview.isActive).toBe(true);
  });

  it("does not reuse a pinned diff for the same path from a different PR", () => {
    actions.addFileDiffPanel(SHARED_PATH, {
      pin: true,
      repositoryName: "backend",
      prKey: "acme/backend/41",
    });

    actions.addFileDiffPanel(SHARED_PATH, {
      repositoryName: "backend",
      prKey: "acme/backend/42",
    });

    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.prKey).toBe("acme/backend/42");
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

  // Regression: a saved env layout can restore `preview:file-diff` into the
  // right column. A subsequent click on a file in the Changes panel should
  // relocate the preview into the explicitly requested (center) group rather
  // than silently reusing the restored slot.
  it("moves an existing preview to the requested group when groupId differs", () => {
    const rightTopId = "group-right-top";
    ({ api, actions } = build(makeApi({ extraGroupIds: [rightTopId] })));

    actions.addFileDiffPanel(PATH_A, { groupId: rightTopId });
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview.group.id).toBe(rightTopId);

    actions.addFileDiffPanel(PATH_B); // defaults to centerGroupId
    expect(preview.group.id).toBe(CENTER_GROUP);
    expect(preview.params.path).toBe(PATH_B);
  });

  it("leaves the preview in place when groupId already matches", () => {
    actions.addFileDiffPanel(PATH_A);
    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview.group.id).toBe(CENTER_GROUP);

    actions.addFileDiffPanel(PATH_B);
    expect(preview.group.id).toBe(CENTER_GROUP);
  });
});

describe("addFileDiffPanel — mixed-layer identity", () => {
  it("does not reuse a pinned diff for another layer of the same file", () => {
    const { api, actions } = build(makeApi());
    actions.addFileDiffPanel(SHARED_PATH, {
      pin: true,
      repositoryName: "frontend",
      changeLayer: "staged",
    });
    actions.addFileDiffPanel(SHARED_PATH, {
      repositoryName: "frontend",
      changeLayer: "unstaged",
    });

    const preview = api.getPanel(PREVIEW_DIFF_ID) as unknown as MockPanel;
    expect(preview).toBeDefined();
    expect(preview.params.changeLayer).toBe("unstaged");
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

  it("serializes remote provenance and keeps repositories with equal SHAs distinct", () => {
    const first = {
      source: "github" as const,
      sha: SHA_A,
      workspaceId: "workspace-1",
      owner: "acme",
      repo: "frontend",
    };
    const second = { ...first, repo: "backend" };

    actions.addCommitDetailPanel(first, { pin: true });
    actions.addCommitDetailPanel(second, { pin: true });

    const firstPanel = api.getPanel(`commit:github:workspace-1:acme/frontend:${SHA_A}`);
    const secondPanel = api.getPanel(`commit:github:workspace-1:acme/backend:${SHA_A}`);
    expect(firstPanel).toBeDefined();
    expect(secondPanel).toBeDefined();
    expect((secondPanel as unknown as MockPanel).params.target).toEqual(second);
    expect((secondPanel as unknown as MockPanel).params.commitSha).toBe(SHA_A);
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
