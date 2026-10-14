// Guided setup for a new coordinator
// (docs/plans/workspace-coordinator-p2/task-07-guided-setup.md).
import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";
import type { Page } from "@playwright/test";
import {
  linkToCoordinatorAdd,
  linkToCoordinatorSettings,
  linkToCoordinatorSettingsList,
} from "../../../lib/coordinator/links";

const SETUP_PATH = /\/coordinators\/setup$/;
const NAME = "Guided Planner";
const GOAL_NAME = "Ship the guided setup";

type SeedData = { workspaceId: string; agentProfileId: string; worktreeExecutorProfileId: string };

async function twoBoards(apiClient: ApiClient, seedData: SeedData) {
  const first = await apiClient.createWorkflow(seedData.workspaceId, "Setup Board One");
  const second = await apiClient.createWorkflow(seedData.workspaceId, "Setup Board Two");
  return { first, second };
}

async function chooseIdentity(page: Page) {
  await page.getByLabel("Name").fill(NAME);
  await page.getByTestId("coordinator-agent-profile-picker").click();
  await page.getByRole("option").first().click();
  await page.getByTestId("executor-profile-selector").first().click();
  await page.getByRole("option").first().click();
}

async function reachReview(page: Page) {
  await page.getByLabel("Name").fill(NAME);
  if (await page.getByTestId("setup-next").isDisabled()) await chooseIdentity(page);
  for (let i = 0; i < 5; i++) await page.getByTestId("setup-next").click();
  await expect(page.getByTestId("setup-review")).toBeVisible();
}

test.describe("Guided setup", () => {
  test("completes six steps with two boards and a goal, Change from Review, then Finish", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { first, second } = await twoBoards(apiClient, seedData);
    await testPage.goto(linkToCoordinatorAdd(seedData.workspaceId));

    await expect(testPage.getByTestId("setup-step-identity")).toHaveAttribute(
      "aria-current",
      "step",
    );
    await expect(testPage.getByTestId("setup-next")).toBeDisabled();
    await testPage.getByLabel("Name").fill(NAME);
    if (await testPage.getByTestId("setup-next").isDisabled()) {
      await chooseIdentity(testPage);
    }
    await testPage.getByTestId("setup-next").click();

    await testPage.getByRole("switch").click();
    const others = (await apiClient.listWorkflows(seedData.workspaceId)).workflows
      .map((w) => w.id)
      .filter((id) => id !== first.id && id !== second.id);
    for (const id of others) {
      const toggle = testPage.getByTestId(`watches-toggle-${id}`);
      if (await toggle.isVisible()) await toggle.click();
    }
    await testPage.getByTestId("setup-next").click();

    await testPage.getByTestId("goal-name").fill(GOAL_NAME);
    await testPage.getByTestId("goal-add-criterion").click();
    await testPage.getByLabel("Exit criterion 1", { exact: true }).fill("Docs shipped");
    await testPage.getByTestId("setup-next").click();

    await testPage.getByLabel("Context").fill("Prefer small cards.");
    await testPage.getByTestId("setup-next").click();
    await testPage.getByTestId("setup-next").click();

    await expect(testPage.getByTestId("setup-review")).toBeVisible();
    await expect(testPage.getByTestId("setup-review-value-watches")).toContainText(
      "Setup Board One, Setup Board Two",
    );
    await expect(testPage.getByTestId("setup-review-value-goal")).toContainText(
      `${GOAL_NAME}, no due date, 1 criterion`,
    );

    await testPage.getByTestId("setup-review-change-name").click();
    await testPage.getByLabel("Name").fill(`${NAME} 2`);
    await testPage.getByTestId("setup-next").click();
    await expect(testPage.getByTestId("setup-review-value-name")).toContainText(`${NAME} 2`);

    const created = waitForHttp(testPage, "POST", SETUP_PATH);
    await testPage.getByTestId("setup-finish").click();
    await created;
    await expect(testPage).toHaveURL(/\/coordinators\/[^/]+$/);

    const list = await apiClient.listCoordinators(seedData.workspaceId);
    const coordinator = list.coordinators.find((c) => c.name === `${NAME} 2`);
    expect(coordinator).toBeDefined();
    const base = linkToCoordinatorSettings(seedData.workspaceId, coordinator?.id ?? "");
    await testPage.goto(`${base}?section=goal`);
    await expect(testPage.getByTestId("goal-name")).toHaveValue(GOAL_NAME);
    await testPage.goto(`${base}?section=watches`);
    await expect(testPage.getByTestId(`watches-board-${first.id}`)).toContainText("In scope");
    await expect(testPage.getByTestId(`watches-board-${second.id}`)).toContainText("In scope");
  });

  test("a 400 returns to its step with values kept", async ({ testPage, seedData }) => {
    await testPage.route(SETUP_PATH, (route) =>
      route.fulfill({
        status: 400,
        contentType: "application/json",
        body: JSON.stringify({ error: "nope", step: "identity", field: "name" }),
      }),
    );
    await testPage.goto(linkToCoordinatorAdd(seedData.workspaceId));
    await reachReview(testPage);
    const answered = waitForHttp(testPage, "POST", SETUP_PATH);
    await testPage.getByTestId("setup-finish").click();
    await answered;
    await expect(testPage.getByTestId("setup-step-identity")).toHaveAttribute(
      "aria-current",
      "step",
    );
    await expect(testPage.getByLabel("Name")).toHaveValue(NAME);
    await expect(testPage.getByTestId("setup-next")).toBeDisabled();
  });

  test("a dropped response says it could not confirm and stays on Review", async ({
    testPage,
    seedData,
  }) => {
    await testPage.route(SETUP_PATH, (route) => route.abort("failed"));
    await testPage.goto(linkToCoordinatorAdd(seedData.workspaceId));
    await reachReview(testPage);
    await testPage.getByTestId("setup-finish").click();
    await expect(testPage.getByTestId("setup-banner-unconfirmed")).toBeVisible();
    await expect(testPage.getByTestId("setup-review")).toBeVisible();
    await expect(testPage.getByTestId("setup-finish")).toBeEnabled();
  });

  test("leaving mid-setup creates nothing", async ({ testPage, apiClient, seedData }) => {
    const before = (await apiClient.listCoordinators(seedData.workspaceId)).coordinators.length;
    await testPage.goto(linkToCoordinatorAdd(seedData.workspaceId));
    await testPage.getByLabel("Name").fill("Never Created");
    let posted = false;
    testPage.on("request", (req) => {
      if (req.method() === "POST" && SETUP_PATH.test(req.url())) posted = true;
    });
    await testPage.goto(linkToCoordinatorSettingsList(seedData.workspaceId));
    expect(posted).toBe(false);
    const after = (await apiClient.listCoordinators(seedData.workspaceId)).coordinators.length;
    expect(after).toBe(before);
  });
});
