import { afterEach, beforeEach, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://api.test" }),
}));

import { createWorkspaceCoordinatorGrant } from "./coordinator-api";

const fetchSpy = vi.fn<(...args: [RequestInfo | URL, RequestInit?]) => Promise<Response>>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

it("serializes the workflow scope identifier for a coordinator grant", async () => {
  fetchSpy.mockResolvedValueOnce(
    new Response(JSON.stringify({ grant: { id: "grant-1" } }), {
      status: 201,
      headers: { "Content-Type": "application/json" },
    }),
  );

  await createWorkspaceCoordinatorGrant("workspace-1", {
    coordinator_task_id: "task-1",
    scope_kind: "workflow",
    scope_id: "workflow-1",
    capabilities: "inspect",
  });

  const [, init] = fetchSpy.mock.calls[0] ?? [];
  expect(JSON.parse(String(init?.body))).toMatchObject({
    scope_kind: "workflow",
    scope_id: "workflow-1",
  });
});
