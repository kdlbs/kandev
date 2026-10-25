import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import { installFixturePlugin, uninstallFixturePlugin } from "../../helpers/plugin-fixture";
import {
  invokeCoordinatorAction,
  installAndGrantCoordinator,
  uninstallCoordinator,
} from "./reference-coordinator-helpers";
import {
  installAndGrantObserver,
  invokeObserverAction,
  OBSERVER_PLUGIN_ID,
  readApprovalContext,
  uninstallObserver,
} from "./observer-compatibility-helpers";

test("two independent packages and lifecycle contract", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const taskIds: string[] = [];
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  await installAndGrantObserver(testPage, apiClient, seedData.workspaceId);
  try {
    const instanceKey = `compat-${randomUUID().slice(0, 8)}`;
    await invokeCoordinatorAction(apiClient, seedData.workspaceId, "instance.save", {
      instance_key: instanceKey,
      name: "Compatibility coordinator",
      role: "delivery-lead",
      agent_profile_id: seedData.agentProfileId,
      instructions: "Inspect and claim selected delivery work.",
    });

    const source = await apiClient.createTask(
      seedData.workspaceId,
      `Observer source ${randomUUID().slice(0, 6)}`,
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );
    taskIds.push(source.id);
    await invokeCoordinatorAction(apiClient, seedData.workspaceId, "task.adopt", {
      instance_key: instanceKey,
      task_id: source.id,
      request_id: `claim-${randomUUID()}`,
      reason: "Own the source task while the observer prepares a follow-up proposal.",
    });

    const observerApproval = await readApprovalContext(apiClient, seedData.workspaceId);
    expect(observerApproval.declared_capability_ids).toEqual([
      "host.v2.read:tasks",
      "host.v2.write:tasks",
    ]);
    expect(observerApproval.declared_capability_ids).not.toContain("host.v2.write:execution");
    expect(observerApproval.declared_capability_ids).not.toContain(
      "host.v2.write:managed_agent_conversations",
    );

    await testPage.goto(`/plugins/${OBSERVER_PLUGIN_ID}`);
    const observedSource = testPage.locator(
      `[data-testid="observer-task"][data-task-id="${source.id}"]`,
    );
    await expect(observedSource).toBeVisible();
    await expect(observedSource).toContainText("revision");
    await observedSource.getByTestId(`observer-propose-task-${source.id}`).click();
    await testPage.getByTestId("observer-proposal-title").fill("Compatibility follow-up");
    await testPage
      .getByTestId("observer-proposal-description")
      .fill("Created only after explicit approval.");
    await testPage.getByTestId("observer-proposal-submit").click();
    const proposal = testPage.getByTestId("observer-proposal");
    await expect(proposal).toContainText("Compatibility follow-up");
    const proposalId = await proposal.getAttribute("data-proposal-id");
    expect(proposalId).toBeTruthy();

    const coordinatorProposals = await invokeCoordinatorAction<{ proposals: unknown[] }>(
      apiClient,
      seedData.workspaceId,
      "proposal.list",
      { instance_key: instanceKey },
    );
    expect(coordinatorProposals.proposals).toHaveLength(0);

    const observerCannotAdopt = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${OBSERVER_PLUGIN_ID}/actions/task.adopt`,
      {
        workspaceId: seedData.workspaceId,
        body: { task_id: source.id, request_id: `observer-claim-${randomUUID()}` },
      },
    );
    expect(observerCannotAdopt.status).toBe(404);

    const approve = proposal.getByTestId(`observer-proposal-approve-${proposalId}`);
    await expect(approve).toHaveCSS("min-height", "44px");
    await approve.click();
    await expect(proposal).toContainText("agent not started");
    const list = await apiClient.listTasks(seedData.workspaceId);
    const created = list.tasks.find((task) => task.title === "Compatibility follow-up");
    expect(created).toBeDefined();
    if (!created) throw new Error("Approved observer proposal did not create a Host task");
    taskIds.push(created.id);
    expect((await apiClient.listTaskSessions(created.id)).sessions).toHaveLength(0);

    const disable = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${OBSERVER_PLUGIN_ID}/disable`,
    );
    expect(disable.ok).toBe(true);
    const enable = await apiClient.rawRequest("POST", `/api/plugins/${OBSERVER_PLUGIN_ID}/enable`);
    expect(enable.ok).toBe(true);
    await expect
      .poll(async () => {
        const result = await invokeObserverAction<{
          proposals: Array<{ proposal_id: string; state: string }>;
        }>(apiClient, seedData.workspaceId, "proposal.list");
        return result.proposals.find((item) => item.proposal_id === proposalId)?.state;
      })
      .toBe("created");

    const refreshedApproval = await readApprovalContext(apiClient, seedData.workspaceId);
    const revoke = await apiClient.rawRequest(
      "DELETE",
      `/api/plugins/${OBSERVER_PLUGIN_ID}/capability-approvals`,
      {
        workspace_id: seedData.workspaceId,
        expected_revision: refreshedApproval.approval?.revision ?? 0,
        manifest_digest: refreshedApproval.manifest_digest,
        capability_ids: [],
        reason: "Verify that the observer stops when its task grant is revoked",
        audit_id: `observer-revoke-${randomUUID()}`,
      },
    );
    expect(revoke.ok).toBe(true);
    const revokedRead = await apiClient.rawRequest(
      "POST",
      `/api/plugins/${OBSERVER_PLUGIN_ID}/actions/observe.scan`,
      {
        workspaceId: seedData.workspaceId,
        body: {},
      },
    );
    expect(revokedRead.status).toBe(403);
    const coordinatorState = await invokeCoordinatorAction<{ instances: Array<{ key: string }> }>(
      apiClient,
      seedData.workspaceId,
      "instance.list",
    );
    expect(coordinatorState.instances.map((instance) => instance.key)).toContain(instanceKey);

    await installFixturePlugin(testPage);
    await testPage.goto("/plugins/e2e-hello");
    await expect(testPage.locator("#hello-plugin-page")).toBeVisible();
  } finally {
    await uninstallFixturePlugin(apiClient);
    for (const taskId of taskIds) await apiClient.deleteTask(taskId).catch(() => undefined);
    await uninstallObserver(apiClient);
    await uninstallCoordinator(apiClient);
  }
});
