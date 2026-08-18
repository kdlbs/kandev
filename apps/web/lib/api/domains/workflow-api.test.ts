import { describe, expect, it } from "vitest";
import { afterEach, beforeEach, vi } from "vitest";
import {
  createWorkflowStep,
  getCoordinatorMonitoring,
  normalizeWorkflowTemplate,
  setCoordinatorMonitoring,
} from "./workflow-api";
import { workspaceId } from "@/lib/types/ids";

const fetchSpy = vi.fn<typeof fetch>();

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("normalizeWorkflowTemplate", () => {
  it("preserves template step identities used by transition references", () => {
    const template = normalizeWorkflowTemplate({
      id: "template-1",
      name: "Review flow",
      is_system: true,
      created_at: "",
      updated_at: "",
      default_steps: [
        {
          id: "in-progress",
          name: "In Progress",
          position: 0,
          events: {
            on_turn_complete: [{ type: "move_to_step", config: { step_id: "review" } }],
          },
        },
        { id: "review", name: "Review", position: 1 },
      ],
    });

    expect(template.default_steps?.map((step) => step.id)).toEqual(["in-progress", "review"]);
  });
});

describe("createWorkflowStep", () => {
  it("forwards the cancellation completion policy in the request payload", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "step-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const payload: Parameters<typeof createWorkflowStep>[0] = {
      workflow_id: "workflow-1",
      name: "Working",
      position: 1,
      cancel_triggers_turn_complete: true,
    };
    await createWorkflowStep(payload, { baseUrl: "http://api.test" });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe("http://api.test/api/v1/workflow/steps");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject(payload);
  });
});

const COORDINATOR_MONITORING_ENTRY = {
  workflow_step_id: "step-1",
  selected: true,
  prompt: "watch closely",
};

describe("getCoordinatorMonitoring", () => {
  it("fetches the saved entries for a workflow", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ entries: [COORDINATOR_MONITORING_ENTRY] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const res = await getCoordinatorMonitoring("workflow-1", { baseUrl: "http://api.test" });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe("http://api.test/api/v1/workflows/workflow-1/coordinator-monitoring");
    expect(init?.method ?? "GET").toBe("GET");
    expect(res.entries).toEqual([COORDINATOR_MONITORING_ENTRY]);
  });
});

describe("setCoordinatorMonitoring", () => {
  it("PUTs the workspace id and entries and returns the saved response", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ entries: [COORDINATOR_MONITORING_ENTRY] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const payload = {
      workspace_id: workspaceId("ws-1"),
      entries: [COORDINATOR_MONITORING_ENTRY],
    };
    const res = await setCoordinatorMonitoring("workflow-1", payload, {
      baseUrl: "http://api.test",
    });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe("http://api.test/api/v1/workflows/workflow-1/coordinator-monitoring");
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual(payload);
    expect(res.entries).toEqual(payload.entries);
  });
});
