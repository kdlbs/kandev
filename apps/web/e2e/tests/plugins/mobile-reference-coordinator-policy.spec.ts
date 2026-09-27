import { randomUUID } from "node:crypto";
import { expect, test } from "../../fixtures/test-base";
import {
  installAndGrantCoordinator,
  invokeCoordinatorAction,
  uninstallCoordinator,
} from "./reference-coordinator-helpers";

test("phone coordinator keeps proposal review and approval in the task tab", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await installAndGrantCoordinator(testPage, apiClient, seedData.workspaceId);
  await invokeCoordinatorAction(apiClient, seedData.workspaceId, "instance.save", {
    instance_key: "phone-policy-lead",
    name: "Phone policy lead",
    role: "chief-of-staff",
    agent_profile_id: seedData.agentProfileId,
    instructions: "Wait for explicit proposal approval.",
  });
  await invokeCoordinatorAction(apiClient, seedData.workspaceId, "proposal.create", {
    instance_key: "phone-policy-lead",
    source_id: `phone-e2e:${randomUUID()}`,
    title: "Inspect the phone release checklist",
    description: "Confirm the mobile checklist is complete.",
  });

  await testPage.goto("/plugins/kandev-plugin-coordinator");
  await testPage.getByRole("tab", { name: "Tasks" }).tap();
  const proposals = testPage.getByTestId("coordinator-proposal-list");
  await expect(proposals).toContainText("Inspect the phone release checklist");
  await expect(testPage.getByTestId("coordinator-proposal-approve")).toHaveCSS(
    "min-height",
    "44px",
  );
  await testPage.getByTestId("coordinator-proposal-approve").tap();
  await expect(proposals).toContainText("approved");
  await expect(proposals).toContainText("Task ");

  await testPage.getByTestId("coordinator-open-settings").tap();
  await expect(testPage.getByTestId("coordinator-routines")).toBeVisible();
  await testPage.getByTestId("coordinator-routine-name").fill("Phone release check");
  await testPage
    .getByTestId("coordinator-routine-prompt")
    .fill("Check mobile release readiness and report blockers.");
  await testPage.getByTestId("coordinator-routine-create").tap();
  const routines = testPage.getByTestId("coordinator-routine-list");
  await expect(routines).toContainText("Phone release check");
  await testPage.getByRole("button", { name: "Pause routine" }).tap();
  await expect(routines).toContainText("Paused");
  await testPage.getByRole("button", { name: "Delete routine" }).tap();
  await expect(testPage.getByText("No recurring routines.")).toBeVisible();

  const geometry = await testPage.getByTestId("coordinator-settings").evaluate((element) => ({
    clientWidth: element.clientWidth,
    scrollWidth: element.scrollWidth,
  }));
  expect(geometry.scrollWidth).toBeLessThanOrEqual(geometry.clientWidth + 1);
  await uninstallCoordinator(apiClient);
});
