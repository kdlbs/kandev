import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import { deleteWorkspaceAction, exportAllWorkflowsAction } from "./workspaces";

describe("exportAllWorkflowsAction", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("workflows: []", { status: 200 })),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  const requestedUrl = () => {
    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    expect(fetchMock).toHaveBeenCalledTimes(1);
    return new URL(String(fetchMock.mock.calls[0][0]));
  };

  it("omits the ids param when no workflow IDs are passed (export all)", async () => {
    await exportAllWorkflowsAction("ws-1");
    expect(requestedUrl().searchParams.has("ids")).toBe(false);
  });

  it("restricts the export to the provided workflow IDs", async () => {
    await exportAllWorkflowsAction("ws-1", ["wf-1", "wf-3"]);
    expect(requestedUrl().searchParams.get("ids")).toBe("wf-1,wf-3");
  });

  it("sends an empty ids param so nothing is exported when the set is empty", async () => {
    await exportAllWorkflowsAction("ws-1", []);
    const url = requestedUrl();
    expect(url.searchParams.has("ids")).toBe(true);
    expect(url.searchParams.get("ids")).toBe("");
  });
});

describe("deleteWorkspaceAction", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("", { status: 204 })),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("sends the workspace name as confirm_name in the DELETE body", async () => {
    await deleteWorkspaceAction("ws-1", "My Workspace");

    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    expect(fetchMock).toHaveBeenCalledTimes(1);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://backend.test/api/v1/workspaces/ws-1");
    expect(init.method).toBe("DELETE");
    expect(JSON.parse(init.body as string)).toEqual({ confirm_name: "My Workspace" });
  });
});
