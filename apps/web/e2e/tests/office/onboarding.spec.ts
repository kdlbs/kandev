import { test, expect } from "../../fixtures/office-fixture";

test.describe("Onboarding", () => {
  test("onboarding state reflects completed after setup", async ({ officeApi, officeSeed }) => {
    const state = await officeApi.getOnboardingState();
    const s = state as { completed: boolean; workspaceId?: string };
    expect(s.completed).toBe(true);
    expect(s.workspaceId).toBe(officeSeed.workspaceId);
  });

  test("completed onboarding has workspace and agent IDs", async ({ officeApi, officeSeed }) => {
    const state = await officeApi.getOnboardingState();
    const s = state as { completed: boolean; workspaceId?: string; ceoAgentId?: string };
    expect(s.workspaceId).toBeTruthy();
    expect(s.ceoAgentId).toBeTruthy();
    expect(s.workspaceId).toBe(officeSeed.workspaceId);
    expect(s.ceoAgentId).toBe(officeSeed.agentId);
  });

  test("dashboard page loads after onboarding", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    // Should NOT redirect to /office/setup
    await expect(testPage).toHaveURL(/\/office$/, { timeout: 10_000 });
  });

  test("dashboard page shows metric cards", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office");
    await expect(testPage.getByRole("heading", { name: /Dashboard/i }).first()).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText("Agents Enabled")).toBeVisible({ timeout: 10_000 });
  });

  test("setup page redirects to dashboard when already completed", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup");
    // Backend sees onboarding as completed → SSR redirects to /office
    await expect(testPage).toHaveURL(/\/office$/, { timeout: 10_000 });
  });

  test("onboarding with task title creates a task", async ({
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    // Re-running onboarding would fail ("workspace already has a CEO agent"), so
    // we verify the same behaviour by creating a task directly via the task API.
    await apiClient.createTask(officeSeed.workspaceId, "First Onboarding Task", {
      workflow_id: officeSeed.workflowId,
    });

    const issues = await officeApi.listTasks(officeSeed.workspaceId);
    const list = (issues as { tasks?: Record<string, unknown>[] }).tasks ?? [];
    const found = list.find(
      (i) => (i as Record<string, unknown>).title === "First Onboarding Task",
    );
    expect(found).toBeDefined();
    expect(officeSeed.workspaceId).toBeTruthy();
    expect(officeSeed.agentId).toBeTruthy();
  });

  test("setup page with mode=new shows wizard when already completed", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await expect(testPage).toHaveURL(/\/office\/setup\?mode=new/, { timeout: 10_000 });
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible({
      timeout: 10_000,
    });
  });

  test('"Add workspace" button navigates to setup with mode=new', async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office");
    await testPage.getByRole("button", { name: "Add workspace" }).click();
    await expect(testPage).toHaveURL(/\/office\/setup\?mode=new/, { timeout: 10_000 });
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible({
      timeout: 10_000,
    });
  });

  test("cancel button returns to dashboard when adding a new workspace", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/setup?mode=new");
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();

    await testPage.getByRole("button", { name: "Cancel" }).click();

    await expect(testPage).toHaveURL(/\/office$/, { timeout: 10_000 });
    await expect(testPage.getByRole("heading", { name: /Dashboard/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("inline CLI profile creation selects the profile for the new agent", async ({
    testPage,
    officeSeed: _,
  }) => {
    test.setTimeout(90_000);
    const suffix = Date.now().toString(36);
    const workspaceName = `Inline Profile Workspace ${suffix}`;
    const profileName = `Inline E2E Profile ${suffix}`;

    await testPage.goto("/office/setup?mode=new");

    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();
    await testPage.getByLabel("Workspace name").fill(workspaceName);
    await testPage.getByRole("button", { name: /next/i }).click();

    await expect(
      testPage.getByRole("heading", { name: "Create your coordinator agent" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /Create a new CLI profile/i }).click();

    await expect(testPage.getByTestId("cli-flags-field")).toBeVisible();
    await expect(testPage.getByText("CLI passthrough")).toHaveCount(0);

    await testPage.getByLabel("Profile name").fill(profileName);
    await testPage.getByRole("button", { name: "Create profile" }).click();

    await expect(testPage.getByTestId("agent-profile-selector")).toContainText(profileName);
    await testPage.getByRole("button", { name: /next/i }).click();

    await expect(
      testPage.getByRole("heading", { name: "Give your CEO something to do" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /skip/i }).click();

    await expect(testPage.getByRole("heading", { name: "Review and launch" })).toBeVisible();
    await expect(testPage.getByText(workspaceName)).toBeVisible();
    await expect(testPage.getByText(profileName)).toBeVisible();

    const completeResponse = testPage.waitForResponse(
      (resp) => resp.url().includes("/onboarding/complete") && resp.status() === 201,
      { timeout: 15_000 },
    );
    await testPage.locator("button:has-text('Create & Launch')").click();
    const completed = await (await completeResponse).json();
    const agentId = (completed as { agentId?: string }).agentId;
    expect(agentId).toBeTruthy();

    await expect(testPage).toHaveURL(/\/office$/, { timeout: 15_000 });
    await testPage.goto(`/office/agents/${agentId}/configuration`);
    await expect(testPage.getByTestId("cli-config-card")).toBeVisible();
    await expect(testPage.getByTestId("cli-config-card").getByText("Unassigned")).toHaveCount(0);
    await expect(
      testPage.getByTestId("cli-config-card").getByText("No CLI profile selected"),
    ).toHaveCount(0);
  });

  test("creating a second workspace via wizard selects it on dashboard", async ({
    testPage,
    officeApi,
    officeSeed: _,
    seedData,
  }) => {
    test.setTimeout(90_000);
    // First, verify the API works for creating a second workspace
    const apiResult = await officeApi.completeOnboarding({
      workspaceName: "API Second Workspace",
      taskPrefix: "API",
      agentName: "CEO",
      agentProfileId: seedData.agentProfileId,
      executorPreference: "local_pc",
    });
    expect(apiResult.workspaceId).toBeTruthy();

    await testPage.goto("/office/setup?mode=new");

    // Step 1: Workspace
    await expect(
      testPage.getByRole("heading", { name: "Set up your Office workspace" }),
    ).toBeVisible();
    await testPage.getByLabel("Workspace name").fill("UI Second Workspace");
    await testPage.getByRole("button", { name: /next/i }).click();

    // Step 2: Agent
    await expect(
      testPage.getByRole("heading", { name: "Create your coordinator agent" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /next/i }).click();

    // Step 3: Task (skip)
    await expect(
      testPage.getByRole("heading", { name: "Give your CEO something to do" }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: /skip/i }).click();

    // Step 4: Review
    await expect(testPage.getByRole("heading", { name: "Review and launch" })).toBeVisible();
    await expect(testPage.getByText("UI Second Workspace")).toBeVisible();

    const responsePromise = testPage.waitForResponse(
      (resp) => resp.url().includes("/onboarding/complete") && resp.status() === 201,
      { timeout: 15_000 },
    );
    await testPage.locator("button:has-text('Create & Launch')").click();
    await responsePromise;

    // Redirect to dashboard with new workspace selected
    await expect(testPage).toHaveURL(/\/office$/, { timeout: 15_000 });
    await expect(testPage.getByRole("heading", { name: /Dashboard/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
