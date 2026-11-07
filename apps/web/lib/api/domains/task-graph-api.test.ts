import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getTaskGraph } from "./task-graph-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function graphResponse() {
  return new Response(
    JSON.stringify({
      mode: "chain",
      available_modes: ["chain"],
      focus_task_id: "task-1",
      nodes: [],
      edges: [],
      truncated: false,
    }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  );
}

describe("getTaskGraph", () => {
  it("omits mode so the server picks the default", async () => {
    fetchSpy.mockResolvedValueOnce(graphResponse());
    await expect(
      getTaskGraph("task-1", undefined, { baseUrl: API_BASE_URL }),
    ).resolves.toMatchObject({
      mode: "chain",
    });
    expect(fetchSpy.mock.calls[0][0]).toBe(`${API_BASE_URL}/api/v1/tasks/task-1/graph`);
  });

  it("sends an explicit mode and encodes the task id", async () => {
    fetchSpy.mockResolvedValueOnce(graphResponse());
    await getTaskGraph("a/b", "subtasks", { baseUrl: API_BASE_URL });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${API_BASE_URL}/api/v1/tasks/a%2Fb/graph?mode=subtasks`,
    );
  });
});
