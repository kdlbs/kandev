import { expect, test } from "../../fixtures/office-fixture";
import type { OfficeApiClient } from "../../helpers/office-api-client";

type RunEvent = { event_type: string; payload: string };
type RunDetail = { status: string; events?: RunEvent[] };
type WorkspaceRun = { id: string; causation_id?: string };

// Fires a taskless routine whose mock agent requests a tool permission, waits
// for the run to finish, and returns the recorded permission decisions.
async function runPermissionRoutine(
  officeApi: OfficeApiClient,
  workspaceId: string,
  agentProfileId: string,
  autoApprove: boolean,
): Promise<string[]> {
  const agent = await officeApi.createAgent(workspaceId, {
    name: `Taskless Permission Agent ${Date.now()}`,
    role: "worker",
    agent_profile_id: agentProfileId,
    executor_preference: '{"type":"local_pc"}',
  });
  const agentId = agent.id as string;
  // Auto-approval follows the launched execution profile. This workspace has no
  // provider routing, so that is the Office agent's own profile row.
  if (autoApprove) await officeApi.updateAgent(agentId, { auto_approve: true });
  // AGENTS.md is inlined into the taskless prompt; the mock agent runs the
  // named scenario, which requests a permission before finishing its turn.
  const instructions = await officeApi.rawRequest(
    "PUT",
    `/agents/${agentId}/instructions/AGENTS.md`,
    { content: "e2e:office_scenario(permission-flow)" },
  );
  expect(instructions.ok).toBe(true);

  const routine = await officeApi.createRoutine(workspaceId, {
    name: `Taskless permission ${Date.now()}`,
    description: "Taskless permission regression",
    assignee_agent_profile_id: agentId,
    concurrency_policy: "always_create",
  });
  const fired = await officeApi.runRoutine(routine.id as string);
  expect(fired.status).toBe(200);
  const causationId = ((await fired.json()) as { run: { causation_id?: string } }).run.causation_id;
  expect(causationId).toBeTruthy();

  let runId = "";
  await expect
    .poll(
      async () => {
        const runs = ((await officeApi.listRuns(workspaceId)).runs ?? []) as WorkspaceRun[];
        runId = runs.find((run) => run.causation_id === causationId)?.id ?? "";
        return runId;
      },
      { timeout: 90_000, message: "Waiting for the routine to create an agent run" },
    )
    .not.toBe("");

  const readDetail = async () => {
    const response = await officeApi.rawRequest("GET", `/agents/${agentId}/runs/${runId}`);
    if (!response.ok) throw new Error(`run detail returned ${response.status}`);
    return (await response.json()) as RunDetail;
  };
  await expect
    .poll(async () => (await readDetail()).status, {
      timeout: 180_000,
      intervals: [1_000, 2_000, 5_000],
      message: "Waiting for the taskless run to finish instead of staying claimed",
    })
    .toBe("finished");
  await expect
    .poll(async () => (await officeApi.getAgent(agentId)).status, { timeout: 60_000 })
    .toBe("idle");

  return ((await readDetail()).events ?? [])
    .filter((event) => event.event_type === "permission.decision")
    .map((event) => (JSON.parse(event.payload) as { decision: string }).decision);
}

// AC-OFFICE-TASKLESS-001.1 / .9: no task or session UI exists to answer a
// taskless permission request, so Office answers it and the run finishes.
test.describe("Office taskless routine permissions", () => {
  test("rejects the request when the launched profile does not auto-approve", async ({
    officeApi,
    officeSeed,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const decisions = await runPermissionRoutine(
      officeApi,
      officeSeed.workspaceId,
      seedData.agentProfileId,
      false,
    );
    expect(decisions).toEqual(["rejected"]);
  });

  test("approves the request when the launched profile auto-approves", async ({
    officeApi,
    officeSeed,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const decisions = await runPermissionRoutine(
      officeApi,
      officeSeed.workspaceId,
      seedData.agentProfileId,
      true,
    );
    expect(decisions).toEqual(["approved"]);
  });
});
