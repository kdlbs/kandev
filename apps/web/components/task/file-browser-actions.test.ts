import { beforeEach, describe, expect, it, vi } from "vitest";
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
