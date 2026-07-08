import { describe, it, expect, vi, beforeEach } from "vitest";
import type { FileContentResponse } from "@/lib/types/backend";

const mockRequestFileContent = vi.fn();

vi.mock("@/lib/utils/file-diff", () => ({
  calculateHash: async (s: string) => `h:${s.length}`,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => mockRequestFileContent(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({}),
}));

import {
  buildFileEditorState,
  fetchFileEditorState,
  getPreviewItemIdToRemoveOnReplace,
  isFileEditorPanelAlreadyRestored,
  isRestoreWriteCurrent,
} from "./file-editor-state";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";

const PATH = "src/foo.ts";
const REPO = "enrichment-commons";
const SESSION_ID = "sess-1";

const RESPONSE: FileContentResponse = {
  path: PATH,
  content: "v1",
  is_binary: false,
} as FileContentResponse;

const FAKE_CLIENT = {} as NonNullable<
  ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>
>;

function makeDockApi(panels: Record<string, { params?: Record<string, unknown> }>) {
  return {
    getPanel: vi.fn((id: string) => panels[id]),
  } as unknown as ReturnType<
    typeof import("@/lib/state/dockview-store").useDockviewStore.getState
  >["api"];
}

describe("buildFileEditorState", () => {
  it("carries the repo subpath so subsequent save/sync calls scope to the right repository", async () => {
    // Multi-repo open: opening foo.ts from the "enrichment-commons" repo must
    // record `repo` on the editor state, otherwise later save/sync requests
    // drop it and the backend stats the bare task root → "file not found".
    const state = await buildFileEditorState(PATH, RESPONSE, REPO);
    expect(state.repo).toBe(REPO);
  });

  it("leaves repo undefined for single-repo tasks", async () => {
    const state = await buildFileEditorState(PATH, RESPONSE);
    expect(state.repo).toBeUndefined();
  });
});

describe("fetchFileEditorState", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("returns the built state (with repo) when the session is unchanged", async () => {
    mockRequestFileContent.mockResolvedValueOnce(RESPONSE);
    const ref = { current: SESSION_ID };

    const state = await fetchFileEditorState({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      filePath: PATH,
      repo: REPO,
      activeSessionIdRef: ref,
    });

    expect(mockRequestFileContent).toHaveBeenCalledWith(FAKE_CLIENT, SESSION_ID, PATH, REPO);
    expect(state?.repo).toBe(REPO);
  });

  it("returns null when the active session changed while the fetch was in flight", async () => {
    // User switches tasks mid-fetch: the late response must not be applied to
    // the new session's editor state.
    const ref = { current: SESSION_ID };
    mockRequestFileContent.mockImplementationOnce(async () => {
      ref.current = "sess-2";
      return RESPONSE;
    });

    const state = await fetchFileEditorState({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      filePath: PATH,
      repo: undefined,
      activeSessionIdRef: ref,
    });

    expect(state).toBeNull();
  });

  it("returns null when another file in the same session became the latest request", async () => {
    const fileKey = buildRepoScopedItemId(PATH, REPO);
    const requestToken = { fileKey, generation: 1 };
    const activeRequestRef = { current: requestToken };
    mockRequestFileContent.mockImplementationOnce(async () => {
      activeRequestRef.current = {
        fileKey: buildRepoScopedItemId("src/bar.ts", REPO),
        generation: 2,
      };
      return RESPONSE;
    });

    const state = await fetchFileEditorState({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      filePath: PATH,
      repo: REPO,
      activeSessionIdRef: { current: SESSION_ID },
      activeRequestRef,
      requestToken,
    });

    expect(state).toBeNull();
  });

  it("returns null when a newer request for the same file superseded it", async () => {
    const fileKey = buildRepoScopedItemId(PATH, REPO);
    const requestToken = { fileKey, generation: 1 };
    const activeRequestRef = { current: requestToken };
    mockRequestFileContent.mockImplementationOnce(async () => {
      activeRequestRef.current = { fileKey, generation: 2 };
      return RESPONSE;
    });

    const state = await fetchFileEditorState({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      filePath: PATH,
      repo: REPO,
      activeSessionIdRef: { current: SESSION_ID },
      activeRequestRef,
      requestToken,
    });

    expect(state).toBeNull();
  });
});

describe("isRestoreWriteCurrent", () => {
  it("rejects a restore write when the active session ref changed", () => {
    expect(isRestoreWriteCurrent(SESSION_ID, SESSION_ID, { current: "sess-2" })).toBe(false);
  });

  it("allows a restore write only when both guards still match", () => {
    expect(isRestoreWriteCurrent(SESSION_ID, SESSION_ID, { current: SESSION_ID })).toBe(true);
  });
});

describe("isFileEditorPanelAlreadyRestored", () => {
  it("detects legacy pinned editor ids when restored panel params match the repo file", () => {
    const dockApi = makeDockApi({
      [`file:${PATH}`]: { params: { path: PATH, repo: REPO } },
    });

    expect(isFileEditorPanelAlreadyRestored(dockApi, PATH, REPO)).toBe(true);
  });

  it("ignores a legacy bare-path panel when its repo params do not match", () => {
    const dockApi = makeDockApi({
      [`file:${PATH}`]: { params: { path: PATH, repo: "other-repo" } },
    });

    expect(isFileEditorPanelAlreadyRestored(dockApi, PATH, REPO)).toBe(false);
  });
});

describe("getPreviewItemIdToRemoveOnReplace", () => {
  const previousItemId = "frontend:README.md";
  const nextItemId = "backend:README.md";

  it("returns the previous preview item when an unpinned preview is replaced", () => {
    const dockApi = makeDockApi({
      [PREVIEW_FILE_EDITOR_ID]: { params: { previewItemId: previousItemId } },
    });

    expect(getPreviewItemIdToRemoveOnReplace(dockApi, nextItemId)).toBe(previousItemId);
  });

  it("keeps state when the previous preview was promoted for materialization", () => {
    const dockApi = makeDockApi({
      [PREVIEW_FILE_EDITOR_ID]: { params: { previewItemId: previousItemId, promoted: true } },
    });

    expect(getPreviewItemIdToRemoveOnReplace(dockApi, nextItemId)).toBeNull();
  });

  it("keeps state when a pinned panel owns the previous preview item", () => {
    const dockApi = makeDockApi({
      [PREVIEW_FILE_EDITOR_ID]: { params: { previewItemId: previousItemId } },
      [`file:${previousItemId}`]: {},
    });

    expect(getPreviewItemIdToRemoveOnReplace(dockApi, nextItemId)).toBeNull();
  });

  it("keeps the current preview state when opening an already pinned next item", () => {
    const dockApi = makeDockApi({
      [PREVIEW_FILE_EDITOR_ID]: { params: { previewItemId: previousItemId } },
      [`file:${nextItemId}`]: {},
    });

    expect(getPreviewItemIdToRemoveOnReplace(dockApi, nextItemId)).toBeNull();
  });

  it("does nothing when reopening the same preview item", () => {
    const dockApi = makeDockApi({
      [PREVIEW_FILE_EDITOR_ID]: { params: { previewItemId: previousItemId } },
    });

    expect(getPreviewItemIdToRemoveOnReplace(dockApi, previousItemId)).toBeNull();
  });
});
