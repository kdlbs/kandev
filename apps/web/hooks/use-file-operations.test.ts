import { describe, it, expect, vi, beforeEach } from "vitest";

const requestFileContentMock = vi.fn();
const triggerFileDownloadMock = vi.fn();

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => requestFileContentMock(...args),
}));
vi.mock("@/lib/utils/file-download", () => ({
  triggerFileDownload: (...args: unknown[]) => triggerFileDownloadMock(...args),
}));

import { downloadFileContent } from "./use-file-operations";

const SESSION_ID = "sess-1";
const FAKE_CLIENT = {} as unknown as Parameters<typeof downloadFileContent>[0];

beforeEach(() => {
  requestFileContentMock.mockReset();
  triggerFileDownloadMock.mockReset();
});

describe("downloadFileContent", () => {
  it("fetches file content and triggers browser download using the file basename", async () => {
    requestFileContentMock.mockResolvedValueOnce({
      path: "src/foo/bar.ts",
      content: "hello",
      is_binary: false,
    });

    const result = await downloadFileContent(FAKE_CLIENT, SESSION_ID, "src/foo/bar.ts");

    expect(result).toEqual({ ok: true });
    expect(requestFileContentMock).toHaveBeenCalledWith(FAKE_CLIENT, SESSION_ID, "src/foo/bar.ts");
    expect(triggerFileDownloadMock).toHaveBeenCalledWith({
      fileName: "bar.ts",
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
      fileName: "logo.png",
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
