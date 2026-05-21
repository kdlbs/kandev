import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  createShare,
  previewShare,
  listShares,
  revokeShare,
  type Share,
  type SnapshotPreview,
} from "./share-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<[FetchInput, FetchInit?], Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("createShare", () => {
  it("POSTs to the nested task/session shares URL", async () => {
    const share: Share = {
      id: "s-1",
      url: "https://gist.github.com/u/abc",
      created_at: "2026-05-21T12:00:00.000Z",
      snapshot_size_bytes: 1024,
    };
    fetchSpy.mockResolvedValueOnce(new Response(JSON.stringify(share), { status: 201 }));

    const got = await createShare("t-1", "sess-1");

    expect(got).toEqual(share);
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/tasks/t-1/sessions/sess-1/shares");
    expect(init?.method).toBe("POST");
  });

  it("url-encodes task and session ids", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(JSON.stringify({}), { status: 201 }));
    await createShare("task/with/slashes", "sess-1");
    expect(lastCall().url).toBe(
      "http://api.test/api/v1/tasks/task%2Fwith%2Fslashes/sessions/sess-1/shares",
    );
  });
});

describe("previewShare", () => {
  it("appends dry_run=true so the backend returns the snapshot inline", async () => {
    const snap: SnapshotPreview = {
      version: 1,
      exported_at: "2026-05-21T12:00:00.000Z",
      task: { title: "Hi" },
      session: { started_at: "2026-05-21T11:00:00.000Z" },
      messages: [],
      redaction: { applied_rules: [] },
    };
    fetchSpy.mockResolvedValueOnce(jsonResponse(snap));

    const got = await previewShare("t-1", "sess-1");

    expect(got.task.title).toBe("Hi");
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/tasks/t-1/sessions/sess-1/shares?dry_run=true");
    expect(init?.method).toBe("POST");
  });
});

describe("listShares", () => {
  it("GETs the shares for a session", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ shares: [] }));
    const got = await listShares("t-1", "sess-1");
    expect(got).toEqual({ shares: [] });
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/tasks/t-1/sessions/sess-1/shares");
    expect(init?.method ?? "GET").toBe("GET");
  });
});

describe("revokeShare", () => {
  it("DELETEs by share id", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));
    await revokeShare("s-1");
    const { url, init } = lastCall();
    expect(url).toBe("http://api.test/api/v1/shares/s-1");
    expect(init?.method).toBe("DELETE");
  });

  it("propagates ApiError on non-2xx", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "missing" }), { status: 404 }),
    );
    await expect(revokeShare("s-1")).rejects.toThrow(/missing/);
  });
});
