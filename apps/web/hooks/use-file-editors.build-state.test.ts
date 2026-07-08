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
  isFileEditorPanelAlreadyRestored,
} from "./use-file-editors";

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

    const state = await fetchFileEditorState(FAKE_CLIENT, SESSION_ID, PATH, REPO, ref);

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

    const state = await fetchFileEditorState(FAKE_CLIENT, SESSION_ID, PATH, undefined, ref);

    expect(state).toBeNull();
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
