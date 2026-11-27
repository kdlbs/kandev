import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  addStandingOrder,
  approveProposal,
  createCoordinator,
  deleteCoordinator,
  getCoordinator,
  getCoordinatorProfileUnavailable,
  getGoal,
  getProposal,
  getProposalConflict,
  listCoordinators,
  listCoordinatorStalls,
  listProposals,
  listStandingOrders,
  markGoalMet,
  openConversation,
  patchCoordinator,
  putGoal,
  rejectProposal,
  restoreStandingOrder,
  retireStandingOrder,
  setGoalCriterionDone,
  type Coordinator,
  isCreateTaskProposal,
  type Proposal,
  type WireProposal,
} from "./coordinator-api";

const fetchSpy = vi.fn<typeof fetch>();
const API_BASE_URL = "http://api.test";
const OPTS = { baseUrl: API_BASE_URL };
const WORKSPACE_ID = "ws-1";
const COORDINATOR_ID = "coord-1";
const PROPOSAL_ID = "proposal-1";
const TIMESTAMP = "2026-09-01T00:00:00Z";
const COORDINATORS_PATH = `${API_BASE_URL}/api/v1/workspaces/${WORKSPACE_ID}/coordinators`;
const COORDINATOR_PATH = `${COORDINATORS_PATH}/${COORDINATOR_ID}`;
const PROPOSALS_PATH = `${COORDINATOR_PATH}/proposals`;
const PROPOSAL_PATH = `${PROPOSALS_PATH}/${PROPOSAL_ID}`;

beforeEach(() => {
  fetchSpy.mockReset();
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const coordinator: Coordinator = {
  id: COORDINATOR_ID,
  workspace_id: WORKSPACE_ID,
  name: "Backend coordinator",
  agent_profile_id: "agent-1",
  executor_profile_id: "executor-1",
  task_agent_profile_id: "ap-1",
  task_executor_profile_id: "ep-1",
  context: "Own the backend surface.",
  conversation_task_id: null,
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

const proposal: Proposal = {
  id: PROPOSAL_ID,
  coordinator_id: COORDINATOR_ID,
  workspace_id: WORKSPACE_ID,
  status: "pending",
  spec: {
    title: "Add rate limiting",
    description: "Add a token bucket limiter to the API gateway.",
    rationale: "Prevent abuse of the public endpoints.",
    workflow_id: "workflow-1",
    step_id: "step-1",
    repository_id: "repo-1",
    source_task_id: "task-1",
  },
  final_spec: null,
  claimed_at: null,
  task_id: null,
  error: null,
  reject_reason: null,
  decided_by: null,
  created_at: TIMESTAMP,
  updated_at: TIMESTAMP,
};

describe("listCoordinators", () => {
  it("gets the workspace's coordinators", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ coordinators: [coordinator] }));

    await expect(listCoordinators(WORKSPACE_ID, OPTS)).resolves.toEqual({
      coordinators: [coordinator],
    });

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(COORDINATORS_PATH);
    expect(init?.method).toBeUndefined();
  });
});

describe("createCoordinator", () => {
  it("posts the create request", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(coordinator));

    const createRequest = {
      name: "Backend coordinator",
      agent_profile_id: "agent-1",
      executor_profile_id: "executor-1",
      task_agent_profile_id: "ap-1",
      task_executor_profile_id: "ep-1",
      context: "Own the backend surface.",
    };
    await createCoordinator(WORKSPACE_ID, createRequest, OPTS);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(COORDINATORS_PATH);
    expect(init).toMatchObject({ method: "POST", body: JSON.stringify(createRequest) });
  });
});

describe("getCoordinator", () => {
  it("gets a single coordinator", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(coordinator));

    await expect(getCoordinator(WORKSPACE_ID, COORDINATOR_ID, OPTS)).resolves.toEqual(coordinator);

    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe(COORDINATOR_PATH);
  });
});

describe("patchCoordinator", () => {
  it("sends only the provided fields", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(coordinator));

    await patchCoordinator(WORKSPACE_ID, COORDINATOR_ID, { name: "Renamed" }, OPTS);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(COORDINATOR_PATH);
    expect(init).toMatchObject({ method: "PATCH", body: JSON.stringify({ name: "Renamed" }) });
  });
});

describe("deleteCoordinator", () => {
  it("sends a DELETE request", async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await deleteCoordinator(WORKSPACE_ID, COORDINATOR_ID, OPTS);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(COORDINATOR_PATH);
    expect(init?.method).toBe("DELETE");
  });
});

describe("listProposals", () => {
  it("omits the status query parameter by default", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ proposals: [proposal] }));

    await listProposals(WORKSPACE_ID, COORDINATOR_ID, undefined, OPTS);

    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe(PROPOSALS_PATH);
  });

  it("passes an explicit status query parameter", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ proposals: [] }));

    await listProposals(WORKSPACE_ID, COORDINATOR_ID, "all", OPTS);

    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${PROPOSALS_PATH}?status=all`);
  });
});

describe("getProposal", () => {
  it("gets a single proposal", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(proposal));

    await expect(getProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, OPTS)).resolves.toEqual(
      proposal,
    );

    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe(PROPOSAL_PATH);
  });
});

describe("listCoordinatorStalls", () => {
  it("gets the workspace's stalls", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ stalls: [] }));

    await expect(listCoordinatorStalls(WORKSPACE_ID, OPTS)).resolves.toEqual({ stalls: [] });

    const [url] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${API_BASE_URL}/api/v1/workspaces/${WORKSPACE_ID}/coordinator-stalls`);
  });
});

describe("approveProposal", () => {
  it("resolves to the approved proposal on 200", async () => {
    const approved: Proposal = { ...proposal, status: "approved" };
    fetchSpy.mockResolvedValueOnce(jsonResponse(approved));

    await expect(
      approveProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, { title: "New title" }, OPTS),
    ).resolves.toEqual(approved);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${PROPOSAL_PATH}/approve`);
    expect(init).toMatchObject({
      method: "POST",
      body: JSON.stringify({ title: "New title" }),
    });
  });

  it("throws an ApiError on 409 whose conflict is readable via getProposalConflict", async () => {
    const settled: Proposal = { ...proposal, status: "rejected" };
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        { error: "proposal_conflict", error_code: "proposal_conflict", proposal: settled },
        409,
      ),
    );

    let error: unknown;
    try {
      await approveProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, undefined, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(ApiError);
    expect(getProposalConflict(error)).toEqual(settled);
  });
});

describe("rejectProposal", () => {
  it("resolves to the rejected proposal on 200", async () => {
    const rejected: Proposal = { ...proposal, status: "rejected", reject_reason: "Not needed" };
    fetchSpy.mockResolvedValueOnce(jsonResponse(rejected));

    await expect(
      rejectProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, "Not needed", OPTS),
    ).resolves.toEqual(rejected);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${PROPOSAL_PATH}/reject`);
    expect(init).toMatchObject({
      method: "POST",
      body: JSON.stringify({ reason: "Not needed" }),
    });
  });

  it("throws an ApiError on 409 whose conflict is readable via getProposalConflict", async () => {
    const settled: Proposal = { ...proposal, status: "approved" };
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        { error: "proposal_conflict", error_code: "proposal_conflict", proposal: settled },
        409,
      ),
    );

    let error: unknown;
    try {
      await rejectProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, undefined, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(ApiError);
    expect(getProposalConflict(error)).toEqual(settled);
  });
});

describe("getProposalConflict", () => {
  it("returns null for a 400 error", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "title is required", field: "title" }, 400),
    );

    let error: unknown;
    try {
      await approveProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, undefined, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getProposalConflict(error)).toBeNull();
  });

  it("returns null for a 409 with a different error_code", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          error: "coordinator profile unavailable",
          error_code: "coordinator_profile_unavailable",
          agent_profile_status: "missing",
          executor_profile_status: "ok",
        },
        409,
      ),
    );

    let error: unknown;
    try {
      await approveProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, undefined, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getProposalConflict(error)).toBeNull();
  });

  it("returns null for a non-ApiError", () => {
    expect(getProposalConflict(new Error("network down"))).toBeNull();
  });

  it("returns null when the 409 body's proposal field is not an object", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        { error: "proposal_conflict", error_code: "proposal_conflict", proposal: "not-an-object" },
        409,
      ),
    );

    let error: unknown;
    try {
      await approveProposal(WORKSPACE_ID, COORDINATOR_ID, PROPOSAL_ID, undefined, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getProposalConflict(error)).toBeNull();
  });
});

describe("openConversation", () => {
  it("posts to the conversation route and resolves to the response body", async () => {
    const response = { task_id: "task-1", session_id: "session-1", archive_state: false };
    fetchSpy.mockResolvedValueOnce(jsonResponse(response));

    await expect(openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS)).resolves.toEqual(response);

    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${COORDINATOR_PATH}/conversation`);
    expect(init?.method).toBe("POST");
  });

  it("throws an ApiError on a 409 conversation_conflict", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "conversation_conflict", error_code: "conversation_conflict" }, 409),
    );

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).errorCode).toBe("conversation_conflict");
  });

  it("throws an ApiError on a 409 coordinator_profile_unavailable", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          error: "coordinator_profile_unavailable",
          agent_profile_status: "missing",
          executor_profile_status: "ok",
        },
        409,
      ),
    );

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(error).toBeInstanceOf(ApiError);
    expect((error as ApiError).status).toBe(409);
    expect((error as ApiError).body).toMatchObject({ error: "coordinator_profile_unavailable" });
  });
});

describe("getCoordinatorProfileUnavailable", () => {
  it("reads the two statuses from a 409 coordinator_profile_unavailable body", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse(
        {
          error: "coordinator_profile_unavailable",
          agent_profile_status: "missing",
          executor_profile_status: "ok",
        },
        409,
      ),
    );

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getCoordinatorProfileUnavailable(error)).toEqual({
      error: "coordinator_profile_unavailable",
      agent_profile_status: "missing",
      executor_profile_status: "ok",
    });
  });

  it("returns null for a 409 conversation_conflict", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({ error: "conversation_conflict", error_code: "conversation_conflict" }, 409),
    );

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getCoordinatorProfileUnavailable(error)).toBeNull();
  });

  it("returns null for a non-409 error", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "not found" }, 404));

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getCoordinatorProfileUnavailable(error)).toBeNull();
  });

  it("returns null for a non-ApiError", () => {
    expect(getCoordinatorProfileUnavailable(new Error("network down"))).toBeNull();
  });

  it("returns null when the profile statuses are missing from the body", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: "coordinator_profile_unavailable" }, 409));

    let error: unknown;
    try {
      await openConversation(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    } catch (caught) {
      error = caught;
    }

    expect(getCoordinatorProfileUnavailable(error)).toBeNull();
  });
});

describe("phase-2 wire shapes", () => {
  const base = {
    id: PROPOSAL_ID,
    coordinator_id: COORDINATOR_ID,
    workspace_id: WORKSPACE_ID,
    status: "pending" as const,
    claimed_at: null,
    task_id: null,
    error: null,
    reject_reason: null,
    decided_by: null,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };

  it("narrows create_task and kind-less proposals to the create shape", () => {
    const spec = {
      title: "t",
      description: "",
      rationale: "",
      workflow_id: "",
      step_id: "",
      repository_id: "",
      source_task_id: "",
    };
    const legacy: WireProposal = { ...base, spec, final_spec: null };
    const created: WireProposal = { ...base, kind: "create_task", spec, final_spec: null };
    expect(isCreateTaskProposal(legacy)).toBe(true);
    expect(isCreateTaskProposal(created)).toBe(true);
  });

  it("treats a known non-create kind and an unknown kind as other kinds", () => {
    const move: WireProposal = { ...base, kind: "move", spec: { step_id: "s" }, final_spec: null };
    const future: WireProposal = { ...base, kind: "reassign", spec: {}, final_spec: null };
    expect(isCreateTaskProposal(move)).toBe(false);
    expect(isCreateTaskProposal(future)).toBe(false);
  });

  it("reads the coordinator policy and watches fields", async () => {
    fetchSpy.mockResolvedValueOnce(
      jsonResponse({
        ...coordinator,
        policy: { actions: { create_task: "propose" } },
        policy_revision: 2,
        watches: { scope: "selected", workflow_ids: ["wf-1"] },
      }),
    );
    const got = await getCoordinator(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    expect(got.policy_revision).toBe(2);
    expect(got.watches).toEqual({ scope: "selected", workflow_ids: ["wf-1"] });
  });
});

describe("standing order client", () => {
  const order = {
    id: "o-1",
    number: 1,
    text: "Prefer small cards.",
    created_at: TIMESTAMP,
    created_by: "u-1",
    retired_at: null,
    last_applied_at: null,
  };

  it("lists active orders by default and retired ones on request", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ orders: [order] }));
    await expect(listStandingOrders(WORKSPACE_ID, COORDINATOR_ID, OPTS)).resolves.toEqual({
      orders: [order],
    });
    expect(fetchSpy.mock.calls[0][0]).toBe(`${COORDINATOR_PATH}/standing-orders`);

    fetchSpy.mockResolvedValueOnce(jsonResponse({ orders: [] }));
    await listStandingOrders(WORKSPACE_ID, COORDINATOR_ID, { ...OPTS, includeRetired: true });
    expect(fetchSpy.mock.calls[1][0]).toBe(`${COORDINATOR_PATH}/standing-orders?include=retired`);
  });

  it("posts the add body with the source proposal id when given", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(order, 201));
    await addStandingOrder(
      WORKSPACE_ID,
      COORDINATOR_ID,
      { text: "Prefer small cards.", source_proposal_id: "p-1" },
      OPTS,
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${COORDINATOR_PATH}/standing-orders`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({
      text: "Prefer small cards.",
      source_proposal_id: "p-1",
    });
  });

  it("posts retire and restore to the order's routes", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ ...order, number: null }));
    await retireStandingOrder(WORKSPACE_ID, COORDINATOR_ID, "o-1", OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${COORDINATOR_PATH}/standing-orders/o-1/retire`);
    expect(fetchSpy.mock.calls[0][1]?.method).toBe("POST");

    fetchSpy.mockResolvedValueOnce(jsonResponse(order));
    await restoreStandingOrder(WORKSPACE_ID, COORDINATOR_ID, "o-1", OPTS);
    expect(fetchSpy.mock.calls[1][0]).toBe(`${COORDINATOR_PATH}/standing-orders/o-1/restore`);
  });
});

describe("goal client", () => {
  const goal = {
    id: "g-1",
    coordinator_id: COORDINATOR_ID,
    name: "Ship",
    due_on: null,
    status: "active",
    criteria: [],
    baseline: null,
    set_at: TIMESTAMP,
    met_at: null,
    met_by: null,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };

  it("gets the goal read", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ active: goal, last_met: null, measures: null }));
    const got = await getGoal(WORKSPACE_ID, COORDINATOR_ID, OPTS);
    expect(got.active?.id).toBe("g-1");
    expect(fetchSpy.mock.calls[0][0]).toBe(`${COORDINATOR_PATH}/goal`);
  });

  it("puts the goal body, keeping an explicit null due_on", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(goal));
    await putGoal(
      WORKSPACE_ID,
      COORDINATOR_ID,
      { goal_id: "g-1", name: "Ship", due_on: null, criteria: [{ text: "A" }] },
      OPTS,
    );
    const [url, init] = fetchSpy.mock.calls[0];
    expect(url).toBe(`${COORDINATOR_PATH}/goal`);
    expect(init?.method).toBe("PUT");
    expect(JSON.parse(String(init?.body))).toEqual({
      goal_id: "g-1",
      name: "Ship",
      due_on: null,
      criteria: [{ text: "A" }],
    });
  });

  it("posts a criterion toggle and mark met", async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse(goal));
    await setGoalCriterionDone(WORKSPACE_ID, COORDINATOR_ID, "c-1", true, OPTS);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${COORDINATOR_PATH}/goal/criteria/c-1`);
    expect(JSON.parse(String(fetchSpy.mock.calls[0][1]?.body))).toEqual({ done: true });

    fetchSpy.mockResolvedValueOnce(jsonResponse(goal));
    await markGoalMet(WORKSPACE_ID, COORDINATOR_ID, "g-1", OPTS);
    expect(fetchSpy.mock.calls[1][0]).toBe(`${COORDINATOR_PATH}/goal/met`);
    expect(JSON.parse(String(fetchSpy.mock.calls[1][1]?.body))).toEqual({ goal_id: "g-1" });
  });
});
