import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { exportConfigManifest, exportSelectedConfigZip } from "./office-extended-api";

const WORKSPACE_ID = "ws-1";
const BASE_URL = "";
const MANIFEST_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/config/export/manifest`;
const ZIP_PATH = `/api/v1/office/workspaces/${WORKSPACE_ID}/config/export/zip`;
const AGENT_PATH = ".kandev/agents/agent.yml";

const originalFetch = global.fetch;

describe("office-config-export-api", () => {
  let fetchSpy: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchSpy = vi.fn();
    global.fetch = fetchSpy as unknown as typeof fetch;
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("loads the workspace-scoped export manifest", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          revision: "rev-1",
          files: [{ path: AGENT_PATH, content: "name: Agent" }],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const manifest = await exportConfigManifest(WORKSPACE_ID, { baseUrl: BASE_URL });

    expect(String(fetchSpy.mock.calls[0]![0])).toBe(MANIFEST_PATH);
    expect(manifest.revision).toBe("rev-1");
    expect(manifest.files[0]?.path).toBe(AGENT_PATH);
  });

  it("posts the selected manifest revision and paths for the ZIP", async () => {
    fetchSpy.mockResolvedValueOnce(new Response("zip-bytes", { status: 200 }));

    const blob = await exportSelectedConfigZip(
      WORKSPACE_ID,
      { revision: "rev-1", paths: [AGENT_PATH] },
      { baseUrl: BASE_URL },
    );

    const init = fetchSpy.mock.calls[0]![1] as RequestInit;
    expect(String(fetchSpy.mock.calls[0]![0])).toBe(ZIP_PATH);
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({
      revision: "rev-1",
      paths: [AGENT_PATH],
    });
    expect(await blob.text()).toBe("zip-bytes");
  });

  it("preserves a stale-revision response as an ApiError", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "export changed" }), {
        status: 409,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(
      exportSelectedConfigZip(
        WORKSPACE_ID,
        { revision: "old-revision", paths: [AGENT_PATH] },
        { baseUrl: BASE_URL },
      ),
    ).rejects.toMatchObject({ status: 409, message: "export changed" });
  });
});
