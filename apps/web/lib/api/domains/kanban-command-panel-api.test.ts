import { afterEach, expect, it, vi } from "vitest";
import { listTasksByWorkspace } from "./kanban-api";

afterEach(() => vi.unstubAllGlobals());

it("serializes the selected workflow and command-panel visibility opt-in", async () => {
  const fetchSpy = vi.fn<typeof fetch>().mockResolvedValue(
    new Response(JSON.stringify({ tasks: [], total: 0 }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchSpy);
  const params = { workflowId: "workflow-1", commandPanelVisible: true, page: 1, pageSize: 20 };
  await listTasksByWorkspace("ws-1", params, { baseUrl: "http://api.test" });

  const url = new URL(String(fetchSpy.mock.calls[0][0]));
  expect(url.searchParams.get("workflow_id")).toBe("workflow-1");
  expect(url.searchParams.get("command_panel_visible")).toBe("true");
});
