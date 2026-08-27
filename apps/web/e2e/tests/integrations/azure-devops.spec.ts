import { expect, test } from "../../fixtures/test-base";

const MOCK_STATE = {
  authenticated: true,
  user: { ok: true, id: "user-1", displayName: "Ada Reviewer", email: "ada@example.com" },
  projects: [{ id: "project-1", name: "Platform", url: "https://dev.azure.com/acme/Platform" }],
  teams: [{ id: "team-1", name: "Platform Team", projectId: "project-1", projectName: "Platform" }],
  boards: [
    { id: "board-1", name: "Stories" },
    { id: "board-2", name: "Tasks" },
  ],
  boardSnapshots: {
    "board-1": {
      board: {
        id: "board-1",
        name: "Stories",
        fields: {
          columnField: { referenceName: "System.BoardColumn" },
          doneField: { referenceName: "System.BoardColumnDone" },
          rowField: { referenceName: "System.BoardRow" },
        },
        columns: [
          { id: "todo", name: "To Do" },
          { id: "active", name: "Active", isSplit: true },
          { id: "done", name: "Done" },
        ],
      },
      items: [
        {
          id: 101,
          revision: 3,
          title: "Handle token rotation",
          description: "<p>Rotate the credentials safely.</p><script>window.bad = true</script>",
          state: "Active",
          type: "User Story",
          project: "project-1",
          assignedTo: "Ada Reviewer",
          tags: ["security"],
          webUrl: "https://dev.azure.com/acme/Platform/_workitems/edit/101",
          columnId: "todo",
          columnDone: false,
        },
      ],
    },
    "board-2": {
      board: {
        id: "board-2",
        name: "Tasks",
        fields: {
          columnField: { referenceName: "System.BoardColumn" },
          doneField: { referenceName: "System.BoardColumnDone" },
          rowField: { referenceName: "System.BoardRow" },
        },
        columns: [
          { id: "todo", name: "To Do" },
          { id: "done", name: "Done" },
        ],
      },
      items: [
        {
          id: 102,
          revision: 1,
          title: "Plan the next release",
          state: "New",
          type: "Task",
          project: "project-1",
          columnId: "todo",
          columnDone: false,
        },
      ],
    },
  },
  repositories: [
    {
      id: "azure-repo-1",
      name: "api",
      projectId: "project-1",
      projectName: "Platform",
      defaultBranch: "refs/heads/main",
      webUrl: "https://dev.azure.com/acme/Platform/_git/api",
    },
  ],
  workItems: [
    {
      id: 101,
      revision: 3,
      title: "Handle token rotation",
      state: "Active",
      type: "User Story",
      project: "project-1",
      assignedTo: "Ada Reviewer",
      webUrl: "https://dev.azure.com/acme/Platform/_workitems/edit/101",
      description: "<p>Rotate the credentials safely.</p><script>window.bad = true</script>",
      fields: { "Microsoft.VSTS.Scheduling.Effort": 3 },
    },
  ],
  workItemComments: {
    "101": [
      {
        id: 1,
        content: "Discussion from Azure",
        author: { id: "user-2", displayName: "Grace Reviewer" },
      },
    ],
  },
  pullRequests: [
    {
      id: 42,
      title: "Rotate integration credentials",
      status: "active",
      isDraft: false,
      sourceBranch: "refs/heads/feature/rotation",
      targetBranch: "refs/heads/main",
      author: { id: "user-1", displayName: "Ada Reviewer" },
      projectId: "project-1",
      projectName: "Platform",
      repositoryId: "azure-repo-1",
      repositoryName: "api",
      webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
      apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
    },
  ],
  feedback: {
    "42": {
      pullRequest: {
        id: 42,
        title: "Rotate integration credentials",
        status: "active",
        isDraft: false,
        sourceBranch: "refs/heads/feature/rotation",
        targetBranch: "refs/heads/main",
        author: { id: "user-1", displayName: "Ada Reviewer" },
        projectId: "project-1",
        projectName: "Platform",
        repositoryId: "azure-repo-1",
        repositoryName: "api",
        webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
        apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
      },
      reviewers: [
        {
          id: "user-2",
          displayName: "Grace Reviewer",
          vote: 10,
          isRequired: true,
          hasDeclined: false,
        },
      ],
      threads: [],
      linkedWorkItems: [{ id: 101, url: "https://dev.azure.com/acme/_apis/wit/workitems/101" }],
      policies: [{ id: "policy-1", status: "approved", name: "Build", isBlocking: true }],
      reviewState: "approved",
      policyState: "success",
    },
  },
};

test("connects and browses Azure work items, PRs, and feedback", async ({
  apiClient,
  seedData,
  testPage,
  prCapture,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  await testPage.goto(
    `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
  );

  const projectInput = testPage.locator("#azure-devops-project");
  const patInput = testPage.getByTestId("azure-devops-pat");
  await expect(projectInput).toBeVisible();
  const [projectBox, patBox] = await Promise.all([
    projectInput.boundingBox(),
    patInput.boundingBox(),
  ]);
  expect(projectBox).not.toBeNull();
  expect(patBox).not.toBeNull();
  expect(Math.abs(projectBox!.y - patBox!.y)).toBeLessThanOrEqual(1);
  expect(projectBox!.height).toBe(patBox!.height);

  await testPage.getByTestId("azure-devops-organization").fill("https://dev.azure.com/acme");
  await testPage.getByRole("button", { name: "How to create a personal access token" }).hover();
  const createTokenLink = testPage.getByTestId("azure-devops-pat-help").locator(":scope > a");
  await expect(createTokenLink).toHaveAttribute(
    "href",
    "https://dev.azure.com/acme/_usersSettings/tokens",
  );
  await expect(testPage.getByTestId("azure-devops-pat-help")).toContainText(
    "Work Items, check Read & write",
  );
  await expect(testPage.getByTestId("azure-devops-pat-help")).toContainText("Code, check Read");
  await testPage.getByTestId("azure-devops-pat").fill("azure-test-pat");
  await testPage.getByTestId("azure-devops-test-button").click();
  await expect(testPage.getByTestId("azure-devops-test-result")).toContainText(
    "Connected as Ada Reviewer",
  );
  await testPage.getByTestId("azure-devops-save-button").click();
  const defaultQueries = testPage
    .getByRole("heading", { name: "Default queries" })
    .locator("xpath=ancestor::section");
  await defaultQueries.getByRole("tab", { name: "Work items" }).click();
  await defaultQueries.getByLabel("Work item query label 1").fill("Team queue");
  await testPage.getByRole("button", { name: "Save changes" }).click();

  await testPage.goto("/azure-devops");
  await expect(testPage.getByTestId("azure-devops-presets-scope-bar")).toContainText("Team queue");
  await expect(testPage.getByTestId("azure-devops-board")).toBeVisible();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();
  await testPage.getByTestId("azure-board-select").click();
  await testPage.getByRole("option", { name: "Tasks" }).click();
  await expect(testPage.getByText("Plan the next release")).toBeVisible();
  await testPage.reload();
  await expect(testPage.getByTestId("azure-board-select")).toContainText("Tasks");
  await expect(testPage.getByText("Plan the next release")).toBeVisible();
  await testPage.getByTestId("azure-board-select").click();
  await testPage.getByRole("option", { name: "Stories" }).click();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();
  await prCapture.screenshot("board-desktop", {
    caption: "Azure DevOps board with columns and cards",
  });

  await testPage.getByTestId("azure-board-card-101").click();
  await expect(testPage.getByTestId("azure-work-item-detail")).toBeVisible();
  await expect(testPage.getByTestId("azure-work-item-detail-description")).toContainText(
    "Rotate the credentials safely.",
  );
  await expect(testPage.getByTestId("azure-work-item-detail-description")).not.toContainText(
    "window.bad",
  );
  await expect(testPage.getByTestId("azure-work-item-detail-comments")).toContainText(
    "Discussion from Azure",
  );
  await testPage.getByTestId("azure-work-item-column").click();
  await testPage.getByRole("option", { name: "Active" }).click();
  await testPage.getByTestId("azure-work-item-column-done").click();
  await testPage.getByRole("option", { name: "Done" }).click();
  await testPage.getByRole("button", { name: "Move" }).click();
  await testPage.getByTestId("azure-work-item-assign-current-user").click();
  await testPage.getByTestId("azure-work-item-detail-close").click();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();

  await testPage
    .getByTestId("azure-board-card-101")
    .dragTo(testPage.getByTestId("azure-board-column-active"));
  await testPage.reload();
  await expect(
    testPage.getByTestId("azure-board-column-active").getByText("Handle token rotation"),
  ).toBeVisible();

  await testPage.getByTestId("azure-devops-work-items-mode").click();
  await testPage.getByTestId("azure-devops-search-button").click();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();
  await testPage.getByTestId("azure-work-item-row-101").click();
  await expect(testPage.getByTestId("azure-work-item-quick-actions")).toBeVisible();
  await testPage.getByRole("button", { name: "Implement" }).click();
  const taskDialog = testPage.getByTestId("create-task-dialog");
  await expect(taskDialog).toBeVisible();
  await expect(taskDialog.getByTestId("task-title-input")).toHaveValue(
    "Implement: Handle token rotation",
  );
  await taskDialog.getByTestId("submit-start-agent-chevron").click();
  await testPage.getByTestId("submit-create-without-agent").click();
  await expect(testPage).toHaveURL(/\/tasks\//);
  await testPage.goto("/azure-devops");
  await testPage.getByTestId("azure-devops-work-items-mode").click();
  await testPage.getByTestId("azure-devops-search-button").click();
  await testPage.getByTestId("azure-work-item-row-101").click();
  await expect(testPage.getByText("Implement: Handle token rotation")).toBeVisible();
  await testPage.getByTestId("azure-work-item-detail-close").click();
  await testPage.getByTestId("azure-devops-pull-requests-mode").click();
  await testPage.getByTestId("azure-devops-search-button").click();
  await expect(testPage.getByText("Rotate integration credentials")).toBeVisible();
  await testPage.getByRole("button", { name: "Feedback" }).click();
  await expect(testPage.getByTestId("azure-devops-feedback-detail")).toContainText(
    "Grace Reviewer",
  );
  await expect(testPage.getByTestId("azure-devops-feedback-detail")).toContainText("Build");

  await testPage.goto(
    `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
  );
  await expect(testPage.getByTestId("azure-devops-watch-settings")).toBeVisible();
  await testPage.getByTestId("azure-add-work-item-watch").click();
  await testPage.getByTestId("azure-work-item-watch-project").fill("project-1");
  await testPage
    .getByTestId("azure-work-item-watch-wiql")
    .fill("SELECT [System.Id] FROM WorkItems");
  await testPage.getByTestId("azure-work-item-watch-repository").fill(seedData.repositoryId);
  await testPage.getByTestId("azure-work-item-watch-workflow").fill(seedData.workflowId);
  await testPage.getByTestId("azure-work-item-watch-step").fill(seedData.startStepId);
  await testPage.getByTestId("azure-work-item-watch-agent").fill(seedData.agentProfileId);
  await testPage
    .getByTestId("azure-work-item-watch-executor")
    .fill(seedData.worktreeExecutorProfileId);
  await testPage.getByRole("button", { name: "Create watch" }).click();
  const workItemWatch = testPage.locator('[data-testid^="azure-work-item-watch-"]').first();
  await expect(workItemWatch).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Run now" }).click();
  await expect(testPage.getByText(/\d+ new match/)).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Disable" }).click();
  await expect(workItemWatch.getByText("Disabled")).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Enable" }).click();
  await expect(workItemWatch.getByText("Enabled")).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Reset" }).click();
  const resetDialog = testPage.getByTestId("reset-watch-dialog");
  await expect(resetDialog).toBeVisible();
  await resetDialog.getByTestId("reset-watch-dialog-confirm").click();
  await expect(testPage.getByText("Watch reset.")).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Delete this watch?" }).click();
  await testPage
    .getByTestId("watcher-delete-confirmation")
    .getByRole("button", { name: "Delete" })
    .click();
  await expect(workItemWatch).toHaveCount(0);

  await testPage.getByTestId("azure-add-pull-request-watch").click();
  await testPage.getByTestId("azure-pull-request-watch-project").fill("project-1");
  await testPage.getByTestId("azure-pull-request-watch-repository").fill(seedData.repositoryId);
  await testPage.getByTestId("azure-pull-request-watch-workflow").fill(seedData.workflowId);
  await testPage.getByTestId("azure-pull-request-watch-step").fill(seedData.startStepId);
  await testPage.getByTestId("azure-pull-request-watch-agent").fill(seedData.agentProfileId);
  await testPage
    .getByTestId("azure-pull-request-watch-executor")
    .fill(seedData.worktreeExecutorProfileId);
  await testPage.getByRole("button", { name: "Create watch" }).click();
  await expect(testPage.getByText("Pull-request filter")).toBeVisible();
});
