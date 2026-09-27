import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  installAndGrantCoordinator,
  invokeCoordinatorAction,
  uninstallCoordinator,
} from "./reference-coordinator-helpers";

test("reference coordinator records and approves one durable proposal", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  await invokeCoordinatorAction(apiClient, seedData.workspaceId, "instance.save", {
    instance_key: "policy-lead",
    name: "Policy lead",
    role: "chief-of-staff",
    agent_profile_id: seedData.agentProfileId,
    instructions: "Review proposals before creating work.",
  });
  await invokeCoordinatorAction(apiClient, seedData.workspaceId, "proposal.create", {
    instance_key: "policy-lead",
    source_id: `e2e:${randomUUID()}`,
    title: "Review the deployment notes",
    description: "Check the release notes and report missing steps.",
  });

  await testPage.goto("/plugins/kandev-plugin-coordinator-template");
  const proposals = testPage.getByTestId("coordinator-proposal-list");
  await expect(proposals).toContainText("Review the deployment notes");
  await proposals.getByTestId("coordinator-proposal-approve").click();

  await expect(proposals).toContainText("approved");
  await expect(proposals).toContainText("Task ");
  const list = await invokeCoordinatorAction<{
    proposals: Array<{ approvalState: string; taskId: string }>;
  }>(apiClient, seedData.workspaceId, "proposal.list", { instance_key: "policy-lead" });
  expect(list.proposals).toHaveLength(1);
  expect(list.proposals[0].approvalState).toBe("approved");
  expect(list.proposals[0].taskId).toBeTruthy();

  const created = await invokeCoordinatorAction<{
    schedule: { id: string; resourceRevision: number; instanceKey: string };
  }>(apiClient, seedData.workspaceId, "routine.create", {
    instance_key: "policy-lead",
    request_id: randomUUID(),
    name: "Yearly release review",
    prompt: "Review the current release checklist and report blockers.",
    triggers: [
      {
        type: "scheduled",
        enabled: true,
        config: { cron_expression: "0 0 1 1 *", timezone: "UTC" },
      },
    ],
  });
  expect(created.schedule.instanceKey).toBe("policy-lead");
  const routines = await invokeCoordinatorAction<{
    schedules: Array<{ id: string; resourceRevision: number }>;
  }>(apiClient, seedData.workspaceId, "routine.list", { instance_key: "policy-lead" });
  expect(routines.schedules.map((schedule) => schedule.id)).toContain(created.schedule.id);
  const deleted = await invokeCoordinatorAction<{ status: string }>(
    apiClient,
    seedData.workspaceId,
    "routine.delete",
    {
      instance_key: "policy-lead",
      schedule_id: created.schedule.id,
      request_id: randomUUID(),
      expected_resource_revision: created.schedule.resourceRevision,
    },
  );
  expect(deleted.status).toBe("APPLIED");
  await uninstallCoordinator(apiClient);
});
