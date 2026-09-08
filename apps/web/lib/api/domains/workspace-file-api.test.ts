import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../client";
import { preflightWorkspaceUpload, uploadWorkspaceFile } from "./workspace-file-api";

const originalFetch = global.fetch;

afterEach(() => {
  global.fetch = originalFetch;
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

describe("preflightWorkspaceUpload", () => {
  it("posts the destination-relative path list and returns the conflicts", async () => {
    const fetchSpy = vi.fn(async () =>
      jsonResponse({ conflicts: [{ path: "fixtures/there.txt", is_dir: false }] }),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const conflicts = await preflightWorkspaceUpload({
      sessionId: "sess-1",
      dir: "fixtures",
      paths: ["there.txt", "missing.txt"],
    });

    expect(conflicts).toEqual([{ path: "fixtures/there.txt", is_dir: false }]);
    const [url, init] = fetchSpy.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/api/v1/task-sessions/sess-1/workspace/files/preflight");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      dir: "fixtures",
      repo: "",
      paths: ["there.txt", "missing.txt"],
    });
  });

  it("treats a missing conflicts array as no conflicts", async () => {
    global.fetch = vi.fn(async () => jsonResponse({})) as typeof global.fetch;

    await expect(
      preflightWorkspaceUpload({ sessionId: "sess-1", dir: "", paths: ["a.txt"] }),
    ).resolves.toEqual([]);
  });

  it("raises ApiError carrying the server message", async () => {
    global.fetch = vi.fn(async () =>
      jsonResponse({ error: "path traversal detected in relative_path" }, 400),
    ) as typeof global.fetch;

    await expect(
      preflightWorkspaceUpload({ sessionId: "sess-1", dir: "", paths: ["../x"] }),
    ).rejects.toMatchObject({ status: 400, message: "path traversal detected in relative_path" });
  });
});

describe("uploadWorkspaceFile", () => {
  it("posts multipart fields and lets the browser set the content type", async () => {
    const fetchSpy = vi.fn(async () =>
      jsonResponse({ path: "fixtures/a.txt", size_bytes: 5, resolution_applied: "" }, 201),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const file = new File(["bytes"], "a.txt");
    const result = await uploadWorkspaceFile({
      sessionId: "sess-1",
      dir: "fixtures",
      relativePath: "a.txt",
      file,
    });

    expect(result.path).toBe("fixtures/a.txt");
    const [url, init] = fetchSpy.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/api/v1/task-sessions/sess-1/workspace/files");
    expect(init.credentials).toBe("include");
    expect((init.headers as Record<string, string> | undefined)?.["Content-Type"]).toBeUndefined();

    const form = init.body as FormData;
    expect(form.get("dir")).toBe("fixtures");
    expect(form.get("relative_path")).toBe("a.txt");
    expect(form.get("size_bytes")).toBe(String(file.size));
    expect(form.get("file")).toBeInstanceOf(File);
    // Absent when there is no conflict, so the server refuses a silent overwrite.
    expect(form.get("resolution")).toBeNull();
  });

  it("carries the resolution when one was chosen", async () => {
    const fetchSpy = vi.fn(async () =>
      jsonResponse(
        { path: "fixtures/a-1.txt", size_bytes: 5, resolution_applied: "keep_both" },
        201,
      ),
    );
    global.fetch = fetchSpy as typeof global.fetch;

    const result = await uploadWorkspaceFile({
      sessionId: "sess-1",
      dir: "fixtures",
      relativePath: "a.txt",
      file: new File(["bytes"], "a.txt"),
      resolution: "keep_both",
    });

    const [, init] = fetchSpy.mock.calls[0] as unknown as [string, RequestInit];
    expect((init.body as FormData).get("resolution")).toBe("keep_both");
    // The server-reported path is authoritative after a rename.
    expect(result.path).toBe("fixtures/a-1.txt");
  });

  it("preserves a 409 so the caller can prompt rather than fail", async () => {
    global.fetch = vi.fn(async () =>
      jsonResponse({ error: "upload destination already exists: a.txt" }, 409),
    ) as typeof global.fetch;

    const error = await uploadWorkspaceFile({
      sessionId: "sess-1",
      dir: "",
      relativePath: "a.txt",
      file: new File(["bytes"], "a.txt"),
    }).catch((e: unknown) => e);

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
  });

  it("falls back to a status diagnostic when the server sends no JSON", async () => {
    global.fetch = vi.fn(
      async () => new Response("upstream exploded", { status: 502 }),
    ) as typeof global.fetch;

    await expect(
      uploadWorkspaceFile({
        sessionId: "sess-1",
        dir: "",
        relativePath: "a.txt",
        file: new File(["bytes"], "a.txt"),
      }),
    ).rejects.toMatchObject({ status: 502 });
  });
});
