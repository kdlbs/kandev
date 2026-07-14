import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor, cleanup } from "@testing-library/react";
import { useRef } from "react";
import type { FileEditorState } from "@/lib/state/dockview-store";
import type { FileInfo } from "@/lib/state/store";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";

const mockRequestFileContent = vi.fn();
const mockGetWebSocketClient = vi.fn();
let openFilesMap = new Map<string, FileEditorState>();

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => mockRequestFileContent(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ openFiles: openFilesMap }),
  },
}));

vi.mock("./use-file-save-delete", () => ({
  updatePanelAfterSave: vi.fn(),
}));

vi.mock("@/lib/utils/file-diff", () => ({
  calculateHash: async (s: string) => `h:${s.length}:${s.slice(0, 8)}`,
}));

import {
  buildGitFileSignature,
  syncOpenFileFromWorkspace,
  useOpenFileWorkspaceSync,
} from "./file-editors-sync";

const FAKE_CLIENT = {} as ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>;
const SESSION_ID = "sess-1";
const PATH = "src/foo.ts";

function fileKey(repo?: string) {
  return buildRepoScopedItemId(PATH, repo);
}

function seedOpenFile(state: Partial<FileEditorState> = {}) {
  const key = fileKey(state.repo);
  openFilesMap = new Map<string, FileEditorState>([
    [
      key,
      {
        path: PATH,
        name: "foo.ts",
        content: "v1",
        originalContent: "v1",
        originalHash: "h:2:v1",
        isDirty: false,
        ...state,
      },
    ],
  ]);
  return key;
}

describe("buildGitFileSignature", () => {
  it("returns a stable __clean__ marker for files absent from git status", () => {
    expect(buildGitFileSignature(undefined)).toBe("__clean__");
  });

  it("changes when the diff content changes (agent-edit detection)", () => {
    const before = buildGitFileSignature({
      path: PATH,
      status: "modified",
      staged: false,
      additions: 1,
      deletions: 0,
      diff: "@@ -1 +1 @@\n-v1\n+v2",
    } as FileInfo);
    const after = buildGitFileSignature({
      path: PATH,
      status: "modified",
      staged: false,
      additions: 1,
      deletions: 0,
      diff: "@@ -1 +1 @@\n-v1\n+v3",
    } as FileInfo);
    expect(before).not.toBe(after);
  });
});

describe("syncOpenFileFromWorkspace", () => {
  let updateFileState: ReturnType<
    typeof vi.fn<(path: string, updates: Partial<FileEditorState>) => void>
  >;

  beforeEach(() => {
    vi.clearAllMocks();
    updateFileState = vi.fn();
  });

  it("replaces editor content when the agent modifies a clean file", async () => {
    // User opens foo.ts (content "v1"). Agent then edits it on disk to "v2".
    // The hook detects the git status change and calls this sync function;
    // we expect the editor buffer to be replaced in place.
    seedOpenFile({ content: "v1", originalContent: "v1", originalHash: "h:2:v1" });
    mockRequestFileContent.mockResolvedValueOnce({
      content: "v2",
      is_binary: false,
      resolved_path: PATH,
    });

    await syncOpenFileFromWorkspace({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      fileKey: PATH,
      path: PATH,
      updateFileState,
    });

    expect(mockRequestFileContent).toHaveBeenCalledWith(FAKE_CLIENT, SESSION_ID, PATH, undefined);
    expect(updateFileState).toHaveBeenCalledTimes(1);
    expect(updateFileState).toHaveBeenCalledWith(
      PATH,
      expect.objectContaining({
        content: "v2",
        originalContent: "v2",
        isDirty: false,
        hasRemoteUpdate: false,
      }),
    );
  });

  it("flags hasRemoteUpdate (instead of clobbering) when the user has unsaved edits", async () => {
    // User has typed local edits (isDirty=true). Agent edits on disk in
    // parallel. We must NOT silently replace the user's buffer; we surface
    // a Reload affordance via hasRemoteUpdate.
    seedOpenFile({
      content: "user-typed",
      originalContent: "v1",
      originalHash: "h:2:v1",
      isDirty: true,
    });
    mockRequestFileContent.mockResolvedValueOnce({
      content: "v2-from-agent",
      is_binary: false,
      resolved_path: PATH,
    });

    await syncOpenFileFromWorkspace({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      fileKey: PATH,
      path: PATH,
      updateFileState,
    });

    expect(updateFileState).toHaveBeenCalledTimes(1);
    expect(updateFileState).toHaveBeenCalledWith(
      PATH,
      expect.objectContaining({
        hasRemoteUpdate: true,
        remoteContent: "v2-from-agent",
      }),
    );
  });

  it("is a no-op when remote content matches the editor buffer", async () => {
    seedOpenFile({ content: "v1", originalContent: "v1", originalHash: "h:2:v1" });
    mockRequestFileContent.mockResolvedValueOnce({
      content: "v1",
      is_binary: false,
      resolved_path: PATH,
    });

    await syncOpenFileFromWorkspace({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      fileKey: PATH,
      path: PATH,
      updateFileState,
    });

    expect(updateFileState).not.toHaveBeenCalled();
  });
});

describe("syncOpenFileFromWorkspace repo scoping", () => {
  let updateFileState: ReturnType<
    typeof vi.fn<(path: string, updates: Partial<FileEditorState>) => void>
  >;

  beforeEach(() => {
    vi.clearAllMocks();
    updateFileState = vi.fn();
  });

  it("forwards the file's repo subpath so multi-repo files resolve under the right repository", async () => {
    // Multi-repo task: foo.ts lives inside the "enrichment-commons" repo. The
    // open editor buffer carries that repo. Re-syncing must pass `repo` to the
    // backend, otherwise it stats <workDir>/src/foo.ts (bare task root) and
    // fails with "file not found" — the reported "Failed to edit" bug.
    const key = seedOpenFile({
      content: "v1",
      originalContent: "v1",
      originalHash: "h:2:v1",
      repo: "enrichment-commons",
    });
    mockRequestFileContent.mockResolvedValueOnce({
      content: "v2",
      is_binary: false,
      resolved_path: PATH,
    });

    await syncOpenFileFromWorkspace({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      fileKey: key,
      path: PATH,
      repo: "enrichment-commons",
      updateFileState,
    });

    expect(mockRequestFileContent).toHaveBeenCalledWith(
      FAKE_CLIENT,
      SESSION_ID,
      PATH,
      "enrichment-commons",
    );
  });

  it("drops the response if the tab was swapped to a different repo mid-fetch", async () => {
    // The fetch is issued for repo "repoA". While it is in flight the same path
    // key is reused for a file from "repoB". Writing repoA's content into the
    // repoB buffer would be wrong, so the stale response must be discarded.
    const key = seedOpenFile({ content: "v1", originalContent: "v1", repo: "repoA" });
    mockRequestFileContent.mockImplementationOnce(async () => {
      openFilesMap = new Map<string, FileEditorState>([
        [
          key,
          {
            path: PATH,
            name: "foo.ts",
            content: "other",
            originalContent: "other",
            originalHash: "h:5:other",
            isDirty: false,
            repo: "repoB",
          },
        ],
      ]);
      return { content: "repoA-content", is_binary: false, resolved_path: PATH };
    });

    await syncOpenFileFromWorkspace({
      client: FAKE_CLIENT,
      sessionId: SESSION_ID,
      fileKey: key,
      path: PATH,
      repo: "repoA",
      updateFileState,
    });

    expect(updateFileState).not.toHaveBeenCalled();
  });
});

function makeStatus(
  files: Record<string, FileInfo>,
  timestamp: string,
  repo?: string,
): GitStatusEntry {
  return {
    branch: "main",
    remote_branch: null,
    modified: [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files,
    timestamp,
    repository_name: repo,
  } as GitStatusEntry;
}

function modifiedFile(diff: string): FileInfo {
  return {
    path: PATH,
    status: "modified",
    staged: false,
    additions: 1,
    deletions: 1,
    diff,
  };
}

type SyncProps = {
  gitStatus: GitStatusEntry | undefined;
  openFiles: Map<string, FileEditorState>;
  updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
};

function renderSyncHook(initial: SyncProps) {
  return renderHook(
    (props: SyncProps) => {
      const activeSessionIdRef = useRef<string | null>(SESSION_ID);
      const gitFileSignaturesRef = useRef<Map<string, string>>(new Map());
      useOpenFileWorkspaceSync({
        gitStatus: props.gitStatus,
        openFiles: props.openFiles,
        updateFileState: props.updateFileState,
        activeSessionIdRef,
        gitFileSignaturesRef,
      });
    },
    { initialProps: initial },
  );
}

describe("useOpenFileWorkspaceSync", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetWebSocketClient.mockReturnValue(FAKE_CLIENT);
  });

  it("refetches the open editor buffer when the agent edits its file", async () => {
    // User has foo.ts open showing "v1". Initial git status has no entry for
    // foo.ts (clean). Then the agent edits the file: a fresh git status arrives
    // with a modified entry. We expect the hook to fetch new content and
    // call updateFileState so the editor displays "v2".
    seedOpenFile({ content: "v1", originalContent: "v1", originalHash: "h:2:v1" });
    const updateFileState = vi.fn();
    mockRequestFileContent.mockResolvedValue({
      content: "v2",
      is_binary: false,
      resolved_path: PATH,
    });

    const initialStatus = makeStatus({}, "2026-05-08T11:00:00.000Z");
    const { rerender } = renderSyncHook({
      gitStatus: initialStatus,
      openFiles: openFilesMap,
      updateFileState,
    });

    // First render baselines the signature; no fetch yet.
    expect(mockRequestFileContent).not.toHaveBeenCalled();

    const editedStatus = makeStatus(
      { [PATH]: modifiedFile("@@ -1 +1 @@\n-v1\n+v2") },
      "2026-05-08T11:00:02.000Z",
    );
    rerender({ gitStatus: editedStatus, openFiles: openFilesMap, updateFileState });

    await waitFor(() => expect(mockRequestFileContent).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(updateFileState).toHaveBeenCalledWith(
        PATH,
        expect.objectContaining({ content: "v2", originalContent: "v2" }),
      ),
    );

    cleanup();
  });

  it("ignores sibling-repo git statuses for an open file at the same path", async () => {
    seedOpenFile({
      content: "user edits",
      originalContent: "v1",
      originalHash: "h:2:v1",
      isDirty: true,
      repo: "frontend",
    });
    const updateFileState = vi.fn();
    mockRequestFileContent.mockResolvedValue({
      content: "backend changed",
      is_binary: false,
      resolved_path: PATH,
    });

    const initialStatus = makeStatus({}, "2026-05-08T11:00:00.000Z", "frontend");
    const { rerender } = renderSyncHook({
      gitStatus: initialStatus,
      openFiles: openFilesMap,
      updateFileState,
    });

    const siblingRepoStatus = makeStatus(
      { [PATH]: modifiedFile("@@ -1 +1 @@\n-v1\n+backend") },
      "2026-05-08T11:00:02.000Z",
      "backend",
    );
    rerender({ gitStatus: siblingRepoStatus, openFiles: openFilesMap, updateFileState });

    await Promise.resolve();
    expect(mockRequestFileContent).not.toHaveBeenCalled();
    expect(updateFileState).not.toHaveBeenCalled();

    cleanup();
  });
});
