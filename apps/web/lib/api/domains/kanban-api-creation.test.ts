import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createTask, fetchTaskByExternalId } from "./kanban-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("native task creation transport", () => {
  it("sends the caller-supplied external identity", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "task-1", deduplicated: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await createTask(
      {
        workspace_id: "ws-1",
        workflow_id: "wf-1",
        title: "Incident task",
        external_id: "ui-task-create/submission-1",
      },
      { baseUrl: API_BASE_URL },
    );

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/tasks`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      external_id: "ui-task-create/submission-1",
    });
  });

  it("looks a task up by encoded external identity without caching", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ id: "task-1", creation_complete: false, external_id: "ui/task 1" }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
    );

    const response = await fetchTaskByExternalId("ws 1", "ui/task 1", {
      baseUrl: API_BASE_URL,
    });

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(
      `${API_BASE_URL}/api/v1/workspaces/ws%201/tasks/by-external-id?external_id=ui%2Ftask+1`,
    );
    expect(init?.cache).toBe("no-store");
    expect(response.creation_complete).toBe(false);
  });
});
