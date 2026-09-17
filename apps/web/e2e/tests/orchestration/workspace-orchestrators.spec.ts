import { test, expect } from "../../fixtures/test-base";
import { resetWorkspaceOrchestrators } from "../../helpers/orchestration";

test("orchestrators connect workspace navigation, profiles, roles and clean conversations", async ({
  testPage,
  backend,
  seedData,
  apiClient,
}) => {
  await backend.restart({ KANDEV_FEATURES_ORCHESTRATION: "true", KANDEV_FEATURES_OFFICE: "false" });
  await resetWorkspaceOrchestrators(testPage.request, backend.baseUrl, seedData.workspaceId);
  await testPage.setViewportSize({ width: 393, height: 852 });
  const officeRequests: string[] = [];
  testPage.on("request", (request) => {
    if (new URL(request.url()).pathname.startsWith("/api/v1/office"))
      officeRequests.push(request.url());
  });
  const ws = seedData.workspaceId;
  const base = `${backend.baseUrl}/api/v1/orchestration`;
  const settings = `/settings/workspaces/${ws}/orchestration`;
  const before = await (
    await testPage.request.get(`${backend.baseUrl}/api/v1/workspaces/${ws}`)
  ).json();
  await testPage.goto("/settings/orchestration");
  await testPage.getByRole("button", { name: "Add role", exact: true }).click();
  await expect(testPage.getByTestId("orchestrator-role-editor")).toHaveCount(2);
  const newRole = testPage.getByTestId("orchestrator-role-editor").last();
  await newRole.getByLabel("Name", { exact: true }).fill("Personal coordinator");
  await newRole
    .getByLabel("Instructions", { exact: true })
    .fill("Coordinate personal projects through task agents.");
  const roleSaved = testPage.waitForResponse(
    (response) => response.url().includes("/roles/") && response.request().method() === "PUT",
  );
  await testPage.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await roleSaved).status()).toBe(200);
  const roles = await (await testPage.request.get(`${base}/roles`)).json();
  const roleId = roles.roles.find(
    (role: { name: string }) => role.name === "Personal coordinator",
  ).id;
  await testPage.goto("/settings/workspaces");
  const card = testPage
    .getByTestId("workspace-list-item")
    .filter({ has: testPage.locator(`a[href="/settings/workspaces/${ws}"]`) });
  await card.getByRole("link", { name: /Orchestration/ }).click();
  await expect(testPage.getByTestId("workspace-orchestrators")).toBeVisible();
  await testPage.getByRole("link", { name: "Add orchestrator", exact: true }).click();
  await expect(testPage.getByLabel("Name", { exact: true })).toHaveCount(0);
  await expect(testPage.getByLabel("Instructions", { exact: true })).toHaveCount(0);
  await testPage.getByTestId("orchestrator-role").click();
  await testPage.getByRole("option", { name: "Personal coordinator", exact: true }).click();
  await testPage.getByTestId("orchestrator-profile").click();
  await testPage.getByRole("option").first().click();
  await testPage.getByTestId("persona-executor-profile").click();
  await testPage.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await testPage.getByRole("button", { name: "Add orchestrator", exact: true }).click();
  await expect(testPage).not.toHaveURL(/\/new$/);
  const firstId = new URL(testPage.url()).pathname.split("/").pop()!;
  const first = await (
    await testPage.request.get(`${base}/workspaces/${ws}/orchestrators/${firstId}`)
  ).json();
  expect(first.profile_id).toBeTruthy();
  const secondRole = await (
    await testPage.request.post(`${base}/roles`, {
      data: { name: "Second coordinator", instructions: "Coordinate other tasks" },
    })
  ).json();
  const created = await testPage.request.post(`${base}/workspaces/${ws}/orchestrators`, {
    data: { ...first, role_id: secondRole.id },
  });
  expect(created.ok()).toBeTruthy();
  await testPage.goto(settings);
  await expect(testPage.getByTestId("orchestrator-card")).toHaveCount(2);
  await testPage
    .getByTestId("orchestrator-card")
    .filter({ hasText: "Personal coordinator" })
    .getByRole("button", { name: "Open conversation" })
    .click();
  await expect(testPage.getByTestId("orchestrator-conversation")).toBeVisible();
  await expect(testPage.getByText("Blocked by", { exact: true })).toHaveCount(0);
  await expect(testPage.getByText("Reviewers", { exact: true })).toHaveCount(0);
  await expect(testPage.getByText("Priority", { exact: true })).toHaveCount(0);
  const conversationTask = new URL(testPage.url()).pathname.split("/").pop()!;
  const commentsUrl = `${base}/tasks/${conversationTask}/comments`;
  await testPage
    .getByTestId("orchestrator-conversation")
    .locator("textarea")
    .fill("Check the workspace and reply briefly.");
  const posted = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/tasks/${conversationTask}/comments`) &&
      response.request().method() === "POST",
  );
  await testPage.getByTestId("orchestrator-conversation").locator("textarea").press("Enter");
  expect((await posted).status()).toBe(201);
  await expect
    .poll(
      async () => {
        const result = await (await testPage.request.get(commentsUrl)).json();
        return result.comments?.some((comment: { source: string }) => comment.source === "session");
      },
      { timeout: 60_000 },
    )
    .toBe(true);
  await expect
    .poll(
      async () => {
        const result = await (await testPage.request.get(commentsUrl)).json();
        return result.comments?.find(
          (comment: { author_type: string }) => comment.author_type === "user",
        )?.run_status;
      },
      { timeout: 60_000 },
    )
    .toBe("finished");
  // Both navigation layouts expose each persona and reopen its existing conversation.
  const conversationUrl = testPage.url();
  await testPage.getByTestId("app-nav-trigger").click();
  const drawer = testPage.getByTestId("app-nav-sheet");
  await expect(
    drawer.getByRole("button", { name: "Second coordinator", exact: true }),
  ).toBeVisible();
  await drawer.getByTestId(`orchestrator-chat-${firstId}`).click();
  await expect(drawer).not.toBeVisible();
  await expect(testPage).toHaveURL(conversationUrl);
  await testPage.setViewportSize({ width: 1440, height: 1000 });
  await expect(testPage.getByTestId(`orchestrator-chat-${firstId}`)).toBeVisible();
  await testPage.getByTestId(`orchestrator-chat-${firstId}`).click();
  await expect(testPage).toHaveURL(conversationUrl);
  await testPage.setViewportSize({ width: 393, height: 852 });
  await testPage.getByRole("link", { name: "Configure orchestrator", exact: true }).click();
  await expect(testPage).toHaveURL(new RegExp(firstId));
  await testPage
    .getByLabel("Workspace context and delegation guidance", { exact: true })
    .fill("Use personal profiles for personal projects.");
  const assignmentSaved = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/orchestrators/${firstId}`) && response.request().method() === "PUT",
  );
  await testPage.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await assignmentSaved).status()).toBe(200);
  await testPage.getByRole("link", { name: "Manage roles", exact: true }).click();
  const roleEditor = testPage.locator(`[data-role-id="${roleId}"]`);
  await roleEditor.getByLabel("Name", { exact: true }).fill("Personal coordinator revised");
  await roleEditor.getByLabel("Persona icon", { exact: true }).click();
  await testPage.getByRole("option", { name: "🧭", exact: true }).click();
  const roleUpdated = testPage.waitForResponse(
    (response) =>
      response.url().endsWith(`/roles/${roleId}`) && response.request().method() === "PUT",
  );
  await testPage.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await roleUpdated).status()).toBe(200);
  await testPage.goto(`${settings}/${firstId}`);
  await expect(
    testPage.getByRole("heading", { name: "Personal coordinator revised", exact: true }),
  ).toBeVisible();
  await expect(testPage.getByLabel("Name", { exact: true })).toHaveCount(0);
  await expect(testPage.getByLabel("Persona icon", { exact: true })).toHaveCount(0);
  await expect(
    testPage.getByLabel("Workspace context and delegation guidance", { exact: true }),
  ).toHaveValue("Use personal profiles for personal projects.");
  const revised = await (
    await testPage.request.get(`${base}/workspaces/${ws}/orchestrators/${firstId}`)
  ).json();
  expect(revised.icon).toBe("🧭");
  expect(revised.profile_id).toBe(first.profile_id);
  const task = await apiClient.createTask(ws, "Coordinated board task", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    metadata: { orchestration_chief_id: firstId, orchestration_managed: true },
  });
  await testPage.reload();
  await testPage
    .getByTestId("orchestrated-tasks")
    .getByRole("link", { name: "Coordinated board task" })
    .click();
  await testPage.getByRole("link", { name: "Coordinating orchestrator", exact: true }).click();
  await expect(testPage).toHaveURL(new RegExp(firstId));
  expect(task.id).toBeTruthy();
  await apiClient.updateTaskState(task.id, "COMPLETED");
  await expect
    .poll(
      async () => {
        const result = await (await testPage.request.get(commentsUrl)).json();
        return result.comments?.filter(
          (comment: { source: string }) => comment.source === "session",
        ).length;
      },
      { timeout: 60_000 },
    )
    .toBe(2);

  await testPage.getByRole("link", { name: "Manage roles", exact: true }).click();
  await expect(testPage.getByTestId("orchestration-roles")).toBeVisible();
  const after = await (
    await testPage.request.get(`${backend.baseUrl}/api/v1/workspaces/${ws}`)
  ).json();
  expect(after.office_workflow_id).toEqual(before.office_workflow_id);
  expect(officeRequests).toEqual([]);
  const otherWorkspace = await apiClient.createWorkspace("Other orchestration scope");
  await testPage.setViewportSize({ width: 1440, height: 1000 });
  await testPage.goto(`/?home=overview&workspaceId=${otherWorkspace.id}`);
  await expect(
    testPage
      .getByTestId("workspace-orchestration-nav")
      .getByRole("link", { name: "Orchestration", exact: true }),
  ).toHaveAttribute("href", `/settings/workspaces/${otherWorkspace.id}/orchestration`);
  await expect(testPage.getByTestId(`orchestrator-chat-${firstId}`)).toHaveCount(0);
  for (const enabled of [true, false]) {
    await backend.restart({
      KANDEV_FEATURES_ORCHESTRATION: String(enabled),
      KANDEV_FEATURES_OFFICE: "true",
    });
    expect((await testPage.request.get(`${base}/workspaces/${ws}/orchestrators`)).status()).toBe(
      enabled ? 200 : 404,
    );
    expect(
      (await testPage.request.get(`${backend.baseUrl}/api/v1/office/agents/${firstId}`)).status(),
    ).toBe(404);
    expect(
      (
        await testPage.request.get(`${backend.baseUrl}/api/v1/office/tasks/${conversationTask}`)
      ).status(),
    ).toBe(404);
    const officeAgents = await (
      await testPage.request.get(`${backend.baseUrl}/api/v1/office/workspaces/${ws}/agents`)
    ).json();
    expect(officeAgents.agents.some((agent: { id: string }) => agent.id === firstId)).toBe(false);
  }
  await backend.restart({
    KANDEV_FEATURES_ORCHESTRATION: "false",
    KANDEV_FEATURES_OFFICE: "false",
  });
  const disabled = await testPage.request.get(`${base}/workspaces/${ws}/orchestrators`);
  expect(disabled.status()).toBe(404);
  await testPage.goto("/settings/workspaces");
  await expect(
    testPage.getByTestId("workspace-section-stats").getByRole("link", { name: /Orchestration/ }),
  ).toHaveCount(0);
});
