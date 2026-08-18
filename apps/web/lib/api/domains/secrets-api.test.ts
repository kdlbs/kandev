import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import {
  copySecret,
  deleteSecret,
  listSecrets,
  moveSecret,
  revealSecret,
  updateSecret,
} from "./secrets-api";

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

  it("copies to the target scope and workspace with the source workspace query", async () => {
    fetchSpy.mockImplementation(async () => new Response("{}", { status: 201 }));
    await copySecret(
      "secret-1",
      { target_scope: "workspace", target_workspace_id: "workspace-2", name: "copied" },
      { workspaceId: WORKSPACE_ID },
    );

    const [requestURL, init] = fetchSpy.mock.calls[0]!;
    const url = new URL(String(requestURL));
    expect(url.pathname).toBe("/api/v1/secrets/secret-1/copy");
    expect(url.searchParams.get("workspace_id")).toBe(WORKSPACE_ID);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      target_scope: "workspace",
      target_workspace_id: "workspace-2",
      name: "copied",
    });
  });

  it("moves to global without emitting a null name", async () => {
    fetchSpy.mockImplementation(async () => new Response("{}", { status: 201 }));
    await moveSecret("secret-1", { target_scope: "global" });

    const [requestURL, init] = fetchSpy.mock.calls[0]!;
    expect(new URL(String(requestURL)).pathname).toBe("/api/v1/secrets/secret-1/move");
    const body = JSON.parse(String(init?.body));
    expect(body.target_scope).toBe("global");
    // The payload builder contract: `name` is undefined (omitted) or a string,
    // never `null`.
    expect(Object.prototype.hasOwnProperty.call(body, "name")).toBe(false);
    expect(body).not.toHaveProperty("name", null);
    expect(body).not.toHaveProperty("target_workspace_id");
  });

  it("surfaces the conflict status for callers", async () => {
    fetchSpy.mockImplementation(
      async () => new Response(JSON.stringify({ error: "conflict" }), { status: 409 }),
    );
    await expect(
      copySecret("secret-1", {
        target_scope: "workspace",
        target_workspace_id: WORKSPACE_ID,
        name: "x",
      }),
    ).rejects.toMatchObject({ status: 409 });
  });
});
