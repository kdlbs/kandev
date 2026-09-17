import { test, expect } from "../../fixtures/office-fixture";

test("workspace chief setup preserves the navigation loop and selection", async ({
  testPage,
  officeSeed,
  backend,
  seedData,
}) => {
  const workspaceId = seedData.workspaceId;
  const beforeResponse = await testPage.request.get(
    `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
  );
  const beforeWorkspace = await beforeResponse.json();
  const settings = `/settings/workspaces/${workspaceId}/agents`;
  await testPage.goto(settings);
  await expect(testPage.getByTestId("workspace-agents-settings")).toBeVisible();
  await testPage.getByRole("button", { name: "Connect agent", exact: true }).click();
  await testPage.getByLabel("Name", { exact: true }).fill("Personal chief");
  await testPage
    .getByRole("dialog")
    .getByLabel("When to use this agent", { exact: true })
    .fill("Use for personal maintenance tasks. Delegate work projects to the work agent.");
  await testPage.getByTestId("connection-profile").click();
  await testPage.getByRole("option").filter({ hasText: "mock" }).first().click();
  const dialog = testPage.getByRole("dialog");
  await dialog.getByRole("combobox").last().click();
  await testPage.getByRole("option").filter({ hasNotText: "Inherit" }).first().click();
  await dialog.getByRole("button", { name: "Connect agent", exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const card = testPage.getByTestId("workspace-agent-card").filter({ hasText: "Personal chief" });
  await card.getByRole("button", { name: "Use as workspace chief" }).click();
  await expect(card.getByText("Primary chief", { exact: true })).toBeVisible();
  await testPage.reload();
  await expect(card.getByText("Primary chief", { exact: true })).toBeVisible();
  await expect(card.getByRole("textbox")).toHaveValue(
    "Use for personal maintenance tasks. Delegate work projects to the work agent.",
  );
  await card.getByTestId("open-agent-conversation").click();
  await expect(testPage).toHaveURL(/\/workspace\/conversations\//);
  const conversation = testPage.url();
  const taskId = new URL(conversation).pathname.split("/").pop();
  const posted = await testPage.request.post(
    `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
    {
      data: { body: "Report readiness using the selected execution profile." },
    },
  );
  expect(posted.ok()).toBeTruthy();
  await expect
    .poll(
      async () => {
        const response = await testPage.request.get(
          `${backend.baseUrl}/api/v1/office/tasks/${taskId}/comments`,
        );
        const { comments } = await response.json();
        return comments.some((comment: { authorType: string }) => comment.authorType === "agent");
      },
      { timeout: 30_000 },
    )
    .toBeTruthy();
  await testPage
    .getByRole("navigation", { name: "Task coordination" })
    .getByRole("link", { name: "Workspace agents" })
    .click();
  await expect(testPage).toHaveURL(new RegExp(settings));
  await card.getByTestId("open-agent-conversation").click();
  await expect(testPage).toHaveURL(conversation);
  await testPage.getByRole("link", { name: "Home", exact: true }).first().click();
  await expect(testPage).toHaveURL(new RegExp(`workspaceId=${workspaceId}`));
  await testPage.getByRole("button", { name: "Personal chief", exact: true }).click();
  await expect(testPage).toHaveURL(conversation);
  const afterResponse = await testPage.request.get(
    `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
  );
  const afterWorkspace = await afterResponse.json();
  expect(afterWorkspace.office_workflow_id).toBe(beforeWorkspace.office_workflow_id);
  const foreign = await testPage.request.put(
    `${backend.baseUrl}/api/v1/office/workspaces/${seedData.workspaceId}/chief`,
    {
      data: { agent_id: officeSeed.agentId },
    },
  );
  expect(foreign.status()).toBe(400);
});

test("Kanban onboarding precedes Office setup and does not create a new workspace", async ({
  testPage,
  officeSeed,
}) => {
  await testPage.addInitScript(() => localStorage.setItem("kandev.onboarding.completed", "false"));
  await testPage.goto("/office/setup");
  await expect(testPage).toHaveURL(/home=overview/);
  await expect(testPage.getByRole("dialog")).toBeVisible();
  await expect(testPage.getByTestId("sidebar-office-button")).not.toBeVisible();
  await testPage.addInitScript(() => localStorage.setItem("kandev.onboarding.completed", "true"));
  await testPage.goto(`/settings/workspaces/${officeSeed.workspaceId}/agents`);
  await expect(testPage.getByTestId("workspace-agents-settings")).toBeVisible();
});
