import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import {
  createQuickTerminalTab,
  deleteQuickTerminalTab,
  listQuickTerminalTabs,
  toQuickTerminalTab,
  updateQuickTerminalTab,
} from "./quick-terminal-api";

type FetchInput = Parameters<typeof fetch>[0];
type FetchInit = Parameters<typeof fetch>[1];
const fetchSpy = vi.fn<(...args: [FetchInput, FetchInit?]) => Promise<Response>>();
const DESCRIPTORS_URL = "http://api.test/api/v1/quick-terminal-tabs";
const WORKSPACE_ID = "workspace-1";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function lastCall(): { url: string; init: FetchInit | undefined } {
  const call = fetchSpy.mock.calls.at(-1);
  if (!call) throw new Error("expected fetch to have been called");
  return { url: String(call[0]), init: call[1] };
}

describe("quick terminal descriptor api", () => {
  it("lists descriptors by workspace and maps the wire shape", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        tabs: [
          {
            tabId: "tab-1",
            workspaceId: WORKSPACE_ID,
            sessionId: "session-1",
            sequence: 2,
            status: "running",
            error: "",
          },
        ],
      }),
    );

    const response = await listQuickTerminalTabs("workspace 1");

    expect(lastCall().url).toBe(
      "http://api.test/api/v1/quick-terminal-tabs?workspace_id=workspace+1",
    );
    expect(response.tabs).toHaveLength(1);
    expect(toQuickTerminalTab(response.tabs[0])).toEqual({
      tabId: "tab-1",
      workspaceId: WORKSPACE_ID,
      sessionId: "session-1",
      sequence: 2,
      status: "running",
    });
  });

  it("persists create, lifecycle, and delete requests independently of chat APIs", async () => {
    const tabId = "6f2d7f2d-0d0c-4c9b-8b73-1c53a5ed5b6b";
    fetchSpy.mockResolvedValueOnce(jsonResponse({ tabId }));
    await createQuickTerminalTab(tabId, WORKSPACE_ID);
    expect(lastCall().url).toBe(DESCRIPTORS_URL);
    expect(lastCall().init?.method).toBe("POST");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      tab_id: tabId,
      workspace_id: WORKSPACE_ID,
    });

    fetchSpy.mockResolvedValueOnce(jsonResponse({ tabId, status: "running" }));
    await updateQuickTerminalTab(tabId, {
      sessionId: "session-1",
      status: "running",
    });
    expect(lastCall().url).toBe(`${DESCRIPTORS_URL}/${tabId}`);
    expect(lastCall().init?.method).toBe("PATCH");
    expect(JSON.parse(String(lastCall().init?.body))).toEqual({
      session_id: "session-1",
      status: "running",
      exit_code: null,
      error: "",
    });

    fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }));
    await deleteQuickTerminalTab(tabId);
    expect(lastCall().url).toBe(`${DESCRIPTORS_URL}/${tabId}`);
    expect(lastCall().init?.method).toBe("DELETE");
  });
});
