import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  cancelCanvasInstall,
  prepareCanvasInstall,
  uploadCanvasInstall,
} from "./canvas-distribution-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];

const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

describe("canvas distribution API", () => {
  it("posts a catalog install request with provenance and archive digest", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ preparation_id: "p1" }));

    await prepareCanvasInstall({
      workspace_id: "workspace-1",
      origin_kind: "registry",
      source_id: "official",
      package_id: "canvas-one",
      expected_version: "1.0.0",
      expected_sha256: "abc",
    });

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/v1/canvases/install-preparations");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      workspace_id: "workspace-1",
      origin_kind: "registry",
      expected_sha256: "abc",
    });
  });

  it("uploads the bundle as multipart without a JSON content type", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ preparation_id: "p1" }));
    const file = new File(["bundle"], "canvas.tar.gz", { type: "application/gzip" });

    await uploadCanvasInstall(file, { workspace_id: "workspace-1", origin_kind: "upload" });

    const [, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(init?.method).toBe("POST");
    expect(init?.body).toBeInstanceOf(FormData);
    expect((init?.body as FormData).get("package")).toBe(file);
    expect(new Headers(init?.headers).get("Content-Type")).toBeNull();
  });

  it("cancels an install preparation by id", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}));

    await cancelCanvasInstall("prep/one");

    const [url, init] = fetchSpy.mock.calls.at(-1) ?? [];
    expect(String(url)).toBe("http://api.test/api/v1/canvases/install-preparations/prep%2Fone");
    expect(init?.method).toBe("DELETE");
  });
});
