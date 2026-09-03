import { describe, expect, it } from "vitest";
import { afterEach, beforeEach, vi } from "vitest";
import { createWorkflowStep, normalizeWorkflowTemplate } from "./workflow-api";

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
          agent_profile_id: "profile-a",
          profile_session_start_policy: "new",
          profile_session_end_policy: "park",
          events: {
            on_turn_complete: [{ type: "move_to_step", config: { step_id: "review" } }],
          },
        },
        { id: "review", name: "Review", position: 1 },
      ],
    });

    expect(template.default_steps?.map((step) => step.id)).toEqual(["in-progress", "review"]);
    expect(template.default_steps?.[0]).toMatchObject({
      agent_profile_id: "profile-a",
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
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
      workflow_id: "workflow-1",
      name: "Working",
      position: 1,
      agent_profile_id: "profile-a",
      profile_session_start_policy: "new",
      profile_session_end_policy: "park",
      cancel_triggers_turn_complete: true,
    };
    await createWorkflowStep(payload, { baseUrl: "http://api.test" });

    expect(fetchSpy).toHaveBeenCalledOnce();
    const [url, init] = fetchSpy.mock.calls[0]!;
    expect(url).toBe("http://api.test/api/v1/workflow/steps");
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toMatchObject(payload);
  });

  it("normalizes both policies on a created step response", async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          id: "step-1",
          workflow_id: "workflow-1",
          name: "Working",
          position: 1,
          color: "",
          profile_session_start_policy: "unsupported",
          profile_session_end_policy: "unsupported",
        }),
        {
          status: 200,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );

    const step = await createWorkflowStep(
      {
        workflow_id: "workflow-1",
        name: "Working",
        position: 1,
        color: "",
      },
      { baseUrl: "http://api.test" },
    );

    expect(step.profile_session_start_policy).toBe("reuse");
    expect(step.profile_session_end_policy).toBe("complete");
  });
});
