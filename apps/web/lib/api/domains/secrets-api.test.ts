import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import { deleteSecret, listSecrets, revealSecret, updateSecret } from "./secrets-api";

describe("secrets API scope parameters", () => {
  const fetchSpy = vi.fn<typeof fetch>();
  const WORKSPACE_ID = "workspace-1";

  beforeEach(() => {
    fetchSpy.mockReset();
    fetchSpy.mockImplementation(async () => new Response("[]", { status: 200 }));
    vi.stubGlobal("fetch", fetchSpy);
  });

  afterEach(() => vi.unstubAllGlobals());

  it("requests workspace secrets with optional global visibility", async () => {
    await listSecrets({
      scope: "workspace",
      workspaceId: WORKSPACE_ID,
      includeGlobal: true,
      cache: "no-store",
    });

    const [requestURL, init] = fetchSpy.mock.calls[0]!;
    const url = new URL(String(requestURL));
    expect(url.pathname).toBe("/api/v1/secrets");
    expect(url.searchParams.get("scope")).toBe("workspace");
    expect(url.searchParams.get("workspace_id")).toBe(WORKSPACE_ID);
    expect(url.searchParams.get("include_global")).toBe("true");
    expect(init?.cache).toBe("no-store");
  });

  it("keeps workspace scope on mutations and reveal", async () => {
    await updateSecret("secret-1", { name: "renamed" }, { workspaceId: WORKSPACE_ID });
    await revealSecret("secret-1", { workspaceId: WORKSPACE_ID });
    await deleteSecret("secret-1", { workspaceId: WORKSPACE_ID });

    expect(fetchSpy).toHaveBeenCalledTimes(3);
    for (const [requestURL] of fetchSpy.mock.calls) {
      expect(new URL(String(requestURL)).searchParams.get("workspace_id")).toBe(WORKSPACE_ID);
    }
  });
});
