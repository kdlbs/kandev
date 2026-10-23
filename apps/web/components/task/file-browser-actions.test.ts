import { beforeEach, describe, expect, it, vi } from "vitest";
import { repositoryId } from "@/lib/types/ids";
import type { FileContentResponse, OpenFileTab } from "@/lib/types/backend";
import type { fetchAndOpenFile as FetchAndOpenFile } from "./file-browser-actions";

const getWebSocketClientMock = vi.fn(() => ({}));
const requestFileContentMock = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => getWebSocketClientMock(),
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileTree: vi.fn(),
  requestFileContent: (...args: unknown[]) => requestFileContentMock(...args),
}));

let fetchAndOpenFile: typeof FetchAndOpenFile;
const SESSION_ID = "session-1";
const FILE_PATH = "src/example.ts";
const REPO = "frontend";
const WORKSPACE_PATH = "/tmp/task-root";
const SECONDARY_ROOT = "/tmp/task-root/secondary";
const SECONDARY_FILE = "secondary/README.md";

beforeEach(async () => {
  vi.resetModules();
  getWebSocketClientMock.mockReset().mockReturnValue({});
  requestFileContentMock.mockReset();
  ({ fetchAndOpenFile } = await import("./file-browser-actions"));
});

function defer<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (reason?: unknown) => void = () => {};
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function response(content = "export const value = 1;"): FileContentResponse {
  return {
    path: FILE_PATH,
    content,
    is_binary: false,
    resolved_path: "target.ts",
  } as FileContentResponse;
}

describe("fetchAndOpenFile", () => {
  it("opens the requested repo-scoped file when the request is current", async () => {
    requestFileContentMock.mockResolvedValueOnce(response());
    const onOpenFile = vi.fn<(tab: OpenFileTab) => void>();
    const toast = vi.fn();

    await fetchAndOpenFile(SESSION_ID, FILE_PATH, onOpenFile, toast, { repo: REPO });

    expect(requestFileContentMock).toHaveBeenCalledWith(
      expect.anything(),
      SESSION_ID,
      FILE_PATH,
      REPO,
    );
    expect(onOpenFile).toHaveBeenCalledWith(
      expect.objectContaining({
        path: FILE_PATH,
        name: "example.ts",
        repo: REPO,
        content: "export const value = 1;",
        isDirty: false,
        resolvedPath: "target.ts",
      }),
    );
    expect(toast).not.toHaveBeenCalled();
  });

  it("does not open a stale file after the request is aborted", async () => {
    const pending = defer<FileContentResponse>();
    requestFileContentMock.mockReturnValueOnce(pending.promise);
    const onOpenFile = vi.fn();
    const toast = vi.fn();
    const controller = new AbortController();

    const opening = fetchAndOpenFile(SESSION_ID, FILE_PATH, onOpenFile, toast, {
      repo: REPO,
      signal: controller.signal,
    });
    expect(requestFileContentMock).toHaveBeenCalledTimes(1);

    controller.abort();
    pending.resolve(response("stale"));
    await opening;

    expect(onOpenFile).not.toHaveBeenCalled();
    expect(toast).not.toHaveBeenCalled();
  });
});

// @covers AC-UI-MARKDOWN-FILE-EDITING-002.13
it("opens a workspace-prefixed secondary checkout with repository-qualified tab identity", async () => {
  requestFileContentMock.mockResolvedValueOnce(response("# Secondary README"));
  const onOpenFile = vi.fn<(tab: OpenFileTab) => void>();
  const scope = {
    workspacePath: WORKSPACE_PATH,
    worktrees: [
      {
        repository_id: repositoryId("primary"),
        worktree_path: "/tmp/task-root/primary",
        position: 0,
        worktree_id: "wt-primary",
      },
      {
        repository_id: repositoryId("secondary-id"),
        worktree_path: SECONDARY_ROOT,
        position: 1,
        worktree_id: "wt-secondary",
      },
    ],
  };
  await fetchAndOpenFile(SESSION_ID, SECONDARY_FILE, onOpenFile, vi.fn(), scope);
  expect(requestFileContentMock).toHaveBeenCalledWith(
    expect.anything(),
    SESSION_ID,
    SECONDARY_FILE,
    undefined,
  );
  expect(onOpenFile).toHaveBeenCalledWith(
    expect.objectContaining({
      path: "README.md",
      repo: "secondary",
      name: "README.md",
      content: "# Secondary README",
    }),
  );
});

it.each([
  { path: "README.md", root: WORKSPACE_PATH, expectedPath: "README.md", expectedRepo: undefined },
  {
    path: "secondary-lookalike/README.md",
    root: SECONDARY_ROOT,
    expectedPath: "secondary-lookalike/README.md",
    expectedRepo: undefined,
  },
  {
    path: SECONDARY_FILE,
    root: "/tmp/outside/secondary",
    expectedPath: SECONDARY_FILE,
    expectedRepo: undefined,
  },
])(
  "preserves an unqualified path without a matching checkout root: $path ($root)",
  async ({ path, root, expectedPath, expectedRepo }) => {
    requestFileContentMock.mockResolvedValueOnce(response());
    const onOpenFile = vi.fn();
    await fetchAndOpenFile(SESSION_ID, path, onOpenFile, vi.fn(), {
      workspacePath: WORKSPACE_PATH,
      worktrees: [
        {
          repository_id: repositoryId("repo"),
          worktree_path: root,
          position: 0,
          worktree_id: "wt",
        },
      ],
    });
    expect(onOpenFile).toHaveBeenCalledWith(
      expect.objectContaining({ path: expectedPath, repo: expectedRepo }),
    );
  },
);

it("keeps explicitly scoped file paths relative to their supplied repository", async () => {
  requestFileContentMock.mockResolvedValueOnce(response());
  const onOpenFile = vi.fn();
  await fetchAndOpenFile(SESSION_ID, SECONDARY_FILE, onOpenFile, vi.fn(), {
    repo: "explicit",
    workspacePath: WORKSPACE_PATH,
    worktrees: [
      {
        repository_id: repositoryId("secondary"),
        worktree_path: SECONDARY_ROOT,
        position: 0,
        worktree_id: "wt",
      },
    ],
  });
  expect(requestFileContentMock).toHaveBeenCalledWith(
    expect.anything(),
    SESSION_ID,
    SECONDARY_FILE,
    "explicit",
  );
  expect(onOpenFile).toHaveBeenCalledWith(
    expect.objectContaining({ path: SECONDARY_FILE, repo: "explicit" }),
  );
});

it("does not infer ownership from ambiguous checkout metadata", async () => {
  requestFileContentMock.mockResolvedValueOnce(response());
  const onOpenFile = vi.fn();
  await fetchAndOpenFile(SESSION_ID, SECONDARY_FILE, onOpenFile, vi.fn(), {
    workspacePath: WORKSPACE_PATH,
    worktrees: [
      {
        repository_id: repositoryId("one"),
        worktree_path: SECONDARY_ROOT,
        position: 0,
        worktree_id: "wt-one",
      },
      {
        repository_id: repositoryId("two"),
        worktree_path: SECONDARY_ROOT,
        position: 1,
        worktree_id: "wt-two",
      },
    ],
  });
  expect(onOpenFile).toHaveBeenCalledWith(
    expect.objectContaining({ path: SECONDARY_FILE, repo: undefined }),
  );
});
