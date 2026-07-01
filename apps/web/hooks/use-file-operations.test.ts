import { describe, it, expect, vi, beforeEach } from "vitest";

const requestFileContentMock = vi.fn();
const triggerFileDownloadMock = vi.fn();

vi.mock("@/lib/ws/workspace-files", () => ({
  createFile: vi.fn(),
  deleteFile: vi.fn(),
  renameFile: vi.fn(),
  requestFileContent: (...args: unknown[]) => requestFileContentMock(...args),
}));
vi.mock("@/lib/utils/file-download", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/utils/file-download")>();
  return {
    ...actual,
    triggerFileDownload: (...args: unknown[]) => triggerFileDownloadMock(...args),
  };
});

import { downloadFileContent } from "./use-file-operations";

const SESSION_ID = "sess-1";
const NESTED_PATH = "src/foo/bar.ts";
const FAKE_CLIENT = {} as unknown as Parameters<typeof downloadFileContent>[0];

beforeEach(() => {
  requestFileContentMock.mockReset();
  triggerFileDownloadMock.mockReset();
});

describe("downloadFileContent", () => {
  it("fetches file content and forwards the full path so the util can derive the basename", async () => {
    requestFileContentMock.mockResolvedValueOnce({
      path: NESTED_PATH,
      content: "hello",
      is_binary: false,
    });

    const result = await downloadFileContent(FAKE_CLIENT, SESSION_ID, NESTED_PATH);

    expect(result).toEqual({ ok: true });
    expect(requestFileContentMock).toHaveBeenCalledWith(FAKE_CLIENT, SESSION_ID, NESTED_PATH);
    expect(triggerFileDownloadMock).toHaveBeenCalledWith({
      fileName: NESTED_PATH,
      content: "hello",
      isBinary: false,
    });
  });

  it("passes isBinary=true through so binary content is decoded correctly", async () => {
    requestFileContentMock.mockResolvedValueOnce({
      path: "assets/logo.png",
      content: "aGk=",
      is_binary: true,
    });

    await downloadFileContent(FAKE_CLIENT, SESSION_ID, "assets/logo.png");

    expect(triggerFileDownloadMock).toHaveBeenCalledWith({
      fileName: "assets/logo.png",
      content: "aGk=",
      isBinary: true,
    });
  });

  it("returns {ok: false, error} when the backend returns an error", async () => {
    requestFileContentMock.mockResolvedValueOnce({
      path: "src/foo.ts",
      content: "",
      error: "Permission denied",
    });

    const result = await downloadFileContent(FAKE_CLIENT, SESSION_ID, "src/foo.ts");

    expect(result).toEqual({ ok: false, error: "Permission denied" });
    expect(triggerFileDownloadMock).not.toHaveBeenCalled();
  });

  it("returns {ok: false} when the request throws", async () => {
    requestFileContentMock.mockRejectedValueOnce(new Error("boom"));

    const result = await downloadFileContent(FAKE_CLIENT, SESSION_ID, "src/foo.ts");

    expect(result.ok).toBe(false);
    if (!result.ok) expect(result.error).toContain("boom");
    expect(triggerFileDownloadMock).not.toHaveBeenCalled();
  });
});
