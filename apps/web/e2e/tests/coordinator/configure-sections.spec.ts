// Sections row, Standing orders and Goal on the coordinator page
// (docs/plans/workspace-coordinator-p2/task-11-orders-goal-sections.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";
import { linkToCoordinatorSettings } from "../../../lib/coordinator/links";

const ORDER_TEXT = "Prefer small cards.";
const GOAL_NAME = "Ship the billing beta";
const ORDERS_PATH = /\/coordinators\/[^/]+\/standing-orders/;
const GOAL_PATH = /\/coordinators\/[^/]+\/goal(\/.*)?$/;

type SeedData = { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string };

function seed(apiClient: ApiClient, seedData: SeedData) {
  return apiClient.createCoordinator(seedData.workspaceId, {
    name: "Sections Coordinator",
    agent_profile_id: seedData.agentProfileId,
    executor_profile_id: seedData.worktreeExecutorProfileId,
  });
}

test.describe("Coordinator page sections", () => {
  test("opens a section from the address and falls back to Identity for an unknown one", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const coordinator = await seed(apiClient, seedData);
    const base = linkToCoordinatorSettings(seedData.workspaceId, coordinator.id);

    await testPage.goto(`${base}?section=goal`);
    const row = testPage.getByRole("tablist", { name: "Coordinator sections" });
    await expect(row).toBeVisible();
    await expect(row.getByRole("tab", { name: "Goal" })).toHaveAttribute("aria-selected", "true");
    await expect(testPage.getByTestId("coordinator-section-help")).toContainText("milestone");

    await testPage.goto(`${base}?section=nonsense`);
    await expect(row.getByRole("tab", { name: "Identity" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    await expect(testPage.getByLabel("Name")).toBeVisible();
  });

  test("adds, retires and undoes a standing order", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);
    const coordinator = await seed(apiClient, seedData);
    await testPage.goto(
      `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=standing-orders`,
    );
    await expect(testPage.getByTestId("standing-orders-empty")).toBeVisible();

    await testPage.getByTestId("standing-order-add").click();
    await testPage.getByTestId("standing-order-text").fill(ORDER_TEXT);
    const added = waitForHttp(testPage, "POST", ORDERS_PATH);
    await testPage.getByTestId("standing-order-save").click();
    await added;
    const row = testPage.getByTestId("standing-order-row");
    await expect(row).toContainText("Standing order 1");
    await expect(row).toContainText(ORDER_TEXT);
    await expect(row).toContainText("Never applied");

    const retired = waitForHttp(testPage, "POST", /standing-orders\/[^/]+\/retire$/);
    await row.getByTestId("standing-order-retire").click();
    await retired;
    await expect(testPage.getByText("Standing order 1 retired.")).toBeVisible();
    await expect(testPage.getByTestId("standing-orders-empty")).toBeVisible();

    const restored = waitForHttp(testPage, "POST", /standing-orders\/[^/]+\/restore$/);
    await testPage.getByRole("button", { name: "Undo" }).click();
    await restored;
    await expect(testPage.getByTestId("standing-order-row")).toContainText(ORDER_TEXT);
  });

  test("sets a goal, checks a criterion and marks it met, and the section survives a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await seed(apiClient, seedData);
    await testPage.goto(
      `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=goal`,
    );

    const setButton = testPage.getByTestId("goal-set");
    await expect(setButton).toBeDisabled();
    await testPage.getByTestId("goal-name").fill(GOAL_NAME);
    await testPage.getByTestId("goal-due").fill("2999-10-31");
    await testPage.getByTestId("goal-add-criterion").click();
    await testPage.getByLabel("Exit criterion 1", { exact: true }).fill("Docs shipped");
    const put = waitForHttp(testPage, "PUT", GOAL_PATH);
    await setButton.click();
    await put;
    await expect(testPage.getByTestId("goal-mark-met")).toBeVisible();
    await expect(testPage.getByTestId("goal-measures")).toBeVisible();

    const toggled = waitForHttp(testPage, "POST", /\/goal\/criteria\/[^/]+$/);
    await testPage.getByLabel("Exit criterion 1 done").click();
    await toggled;

    await testPage.reload();
    await expect(testPage.getByTestId("goal-name")).toHaveValue(GOAL_NAME);
    await expect(testPage.getByLabel("Exit criterion 1 done")).toBeChecked();

    await testPage.getByTestId("goal-mark-met").click();
    const met = waitForHttp(testPage, "POST", /\/goal\/met$/);
    await testPage.getByTestId("goal-mark-met-confirm").click();
    await met;
    await expect(testPage.getByTestId("goal-set")).toBeVisible();

    await testPage.reload();
    await expect(testPage.getByTestId("goal-set")).toBeVisible();
    await expect(testPage.getByTestId("goal-name")).toHaveValue("");
  });
});

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;

test.describe("Coordinator May do and Watches", () => {
  test("saves Message to Requires approval and a narrowed watch set in one PUT, and survives a reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const second = await apiClient.createWorkflow(seedData.workspaceId, "Second board");
    const coordinator = await seed(apiClient, seedData);
    const base = linkToCoordinatorSettings(seedData.workspaceId, coordinator.id);

    await testPage.goto(`${base}?section=watches`);
    await testPage.getByRole("switch").click();
    await testPage.getByTestId(`watches-toggle-${second.id}`).click();

    await testPage.getByRole("tab", { name: "May do" }).click();
    await testPage.locator("#may-do-message-approval").click();

    const put = waitForHttp(testPage, "PUT", SETTINGS_PATH);
    await testPage.getByRole("button", { name: "Save changes" }).click();
    await put;

    await testPage.reload();
    await expect(testPage.locator("#may-do-message-approval")).toBeChecked();
    await testPage.getByRole("tab", { name: "Watches" }).click();
    await expect(testPage.getByTestId(`watches-toggle-${second.id}`)).toHaveText(
      "Put this board in scope",
    );
    await expect(testPage.getByTestId(`watches-toggle-${seedData.workflowId}`)).toHaveText(
      "Take this board out of scope",
    );
  });

  test("the Review link lands on What it did filtered by class", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const coordinator = await seed(apiClient, seedData);
    await testPage.goto(
      `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=may-do`,
    );
    await testPage.getByTestId("may-do-review-message").click();
    await expect(testPage).toHaveURL(/\/queue\?class=message$/);
    await expect(testPage.getByTestId("what-it-did")).toBeInViewport();
  });
});

test.describe("Coordinator page with the phase-2 flag off", () => {
  test("shows the phase-1 page with no Sections row", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_COORDINATOR_PHASE2: "false" });
    try {
      const coordinator = await seed(apiClient, seedData);
      await testPage.goto(linkToCoordinatorSettings(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId("coordinator-editor-page")).toBeVisible();
      await expect(testPage.getByLabel("Name")).toBeVisible();
      await expect(testPage.getByRole("tablist", { name: "Coordinator sections" })).toHaveCount(0);
    } finally {
      await release();
    }
  });
});
