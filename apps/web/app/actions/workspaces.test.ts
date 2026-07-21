import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "http://backend.test" }),
}));

import {
  createWorkflowStepAction,
  deleteWorkspaceAction,
  exportAllWorkflowsAction,
  listWorkflowStepsAction,
  updateWorkflowStepAction,
} from "./workspaces";

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
    expect(url).toBe("http://backend.test/api/v1/office/workspaces/ws-1");
    expect(init.method).toBe("DELETE");
    expect(JSON.parse(init.body as string)).toEqual({ confirm_name: "My Workspace" });
  });
});

describe("workflow step WIP fields", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          id: "step-1",
          workflow_id: "wf-1",
          name: "Review",
          position: 1,
          color: "bg-blue-500",
          wip_limit: 2,
          pull_from_step_id: "step-0",
          created_at: "",
          updated_at: "",
        }),
      ),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("preserves WIP fields returned from workflow step APIs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        Response.json({
          steps: [
            {
              id: "step-1",
              workflow_id: "wf-1",
              name: "Review",
              position: 1,
              color: "bg-blue-500",
              wip_limit: 2,
              pull_from_step_id: "step-0",
              created_at: "",
              updated_at: "",
            },
          ],
        }),
      ),
    );

    const result = await listWorkflowStepsAction("wf-1");

    expect(result.steps[0]).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("sends WIP fields when creating a workflow step", async () => {
    await createWorkflowStepAction({
      workflow_id: "wf-1",
      name: "Review",
      position: 1,
      color: "bg-blue-500",
      wip_limit: 2,
      pull_from_step_id: "step-0",
    } as Parameters<typeof createWorkflowStepAction>[0]);

    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(init.body as string)).toMatchObject({
      wip_limit: 2,
      pull_from_step_id: "step-0",
    });
  });

  it("sends WIP fields when updating a workflow step", async () => {
    await updateWorkflowStepAction("step-1", {
      wip_limit: 3,
      pull_from_step_id: "",
    } as Parameters<typeof updateWorkflowStepAction>[1]);

    const fetchMock = fetch as unknown as ReturnType<typeof vi.fn>;
    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(init.body as string)).toMatchObject({
      wip_limit: 3,
      pull_from_step_id: "",
    });
  });
});
