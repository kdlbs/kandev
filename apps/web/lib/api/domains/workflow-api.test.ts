import { describe, expect, it } from "vitest";
import { afterEach, beforeEach, vi } from "vitest";
import { createWorkflowStep, listWorkflowSteps, normalizeWorkflowTemplate } from "./workflow-api";

const fetchSpy = vi.fn<typeof fetch>();
const apiBaseUrl = "http://api.test";
const requestOptions = { baseUrl: apiBaseUrl };
const workflowId = "workflow-1";

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

describe("normalizeWorkflowTemplate", () => {
  it("preserves template step identities used by transition references", () => {
    const step = {
      id: "in-progress",
      name: "In Progress",
      position: 0,
      agent_profile_id: "profile-a",
      profile_session_start_policy: "new" as const,
      profile_session_end_policy: "park" as const,
      disable_unclassified_fallback: true,
      complete_task_on_enter: false,
      events: {
        on_turn_complete: [{ type: "move_to_step" as const, config: { step_id: "review" } }],
      },
    };
    const template = normalizeWorkflowTemplate({
      id: "template-1",
      name: "Review flow",
      is_system: true,
      created_at: "",
      updated_at: "",
      default_steps: [
        step,
        { id: "review", name: "Review", position: 1, complete_task_on_enter: true },
      ],
    });

    expect(template.default_steps?.map((step) => step.id)).toEqual(["in-progress", "review"]);
    expect(template.default_steps?.[0]).toMatchObject({
      agent_profile_id: "profile-a",
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
      disable_unclassified_fallback: true,
    });
  });
});

describe("createWorkflowStep", () => {
  it("forwards both session lifecycle policies in the request payload", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "step-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const payload: Parameters<typeof createWorkflowStep>[0] = {
      workflow_id: workflowId,
      name: "Working",
      position: 1,
      agent_profile_id: "profile-a",
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
      complete_task_on_enter: false,
      cancel_triggers_turn_complete: true,
    };
    await createWorkflowStep(payload, requestOptions);

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe(`${apiBaseUrl}/api/v1/workflow/steps`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject(payload);
  });

  it("normalizes both policies on a created step response", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "step-1",
          workflow_id: workflowId,
          name: "Working",
          position: 1,
          color: "",
          profile_session_start_policy: "unsupported",
          profile_session_end_policy: "unsupported",
          complete_task_on_enter: false,
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const step = await createWorkflowStep(
      {
        workflow_id: workflowId,
        name: "Working",
        position: 1,
        color: "",
      },
      requestOptions,
    );

    expect(step.profile_session_start_policy).toBe("reuse");
    expect(step.profile_session_end_policy).toBe("park");
    expect(step.complete_task_on_enter).toBe(false);
  });
});

describe("listWorkflowSteps", () => {
  it("normalizes a historical null step list to an empty array", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ steps: null }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(listWorkflowSteps(workflowId, requestOptions)).resolves.toMatchObject({
      steps: [],
    });
  });

  it("normalizes a missing step list to an empty array", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({}), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(listWorkflowSteps(workflowId, requestOptions)).resolves.toMatchObject({
      steps: [],
    });
  });

  it("preserves an empty step array", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ steps: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(listWorkflowSteps(workflowId, requestOptions)).resolves.toMatchObject({
      steps: [],
    });
  });

  it("retains populated steps and normalizes lifecycle defaults", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          steps: [
            {
              id: "step-1",
              workflow_id: workflowId,
              name: "Working",
              position: 0,
              color: "blue",
              profile_session_start_policy: "unsupported",
              profile_session_end_policy: "unsupported",
            },
          ],
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    await expect(listWorkflowSteps(workflowId, requestOptions)).resolves.toMatchObject({
      steps: [
        {
          id: "step-1",
          disable_unclassified_fallback: false,
          profile_session_start_policy: "reuse",
          profile_session_end_policy: "park",
        },
      ],
    });
  });

  it("rejects failed requests instead of returning an empty list", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(JSON.stringify({ error: "Failed to list steps" }), {
        status: 500,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(listWorkflowSteps(workflowId, requestOptions)).rejects.toMatchObject({
      status: 500,
      message: "Failed to list steps",
    });
  });
});
