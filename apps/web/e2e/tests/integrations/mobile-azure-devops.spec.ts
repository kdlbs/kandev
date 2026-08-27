import { expect, test } from "../../fixtures/test-base";

const MOCK_STATE = {
  authenticated: true,
  user: { ok: true, id: "user-1", displayName: "Ada Reviewer" },
  projects: [{ id: "project-1", name: "Platform", url: "https://dev.azure.com/acme/Platform" }],
  teams: [{ id: "team-1", name: "Platform Team", projectId: "project-1", projectName: "Platform" }],
  boards: [{ id: "board-1", name: "Stories" }],
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
          description: "<p>Rotate the credentials safely.</p>",
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
      description: "<p>Rotate the credentials safely.</p>",
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
      linkedWorkItems: [],
      policies: [],
      reviewState: "approved",
      policyState: "success",
    },
  },
};

test("mobile settings explain PAT scopes and link to the organization token page", async ({
  seedData,
  testPage,
}) => {
  await testPage.goto(
    `/settings/workspaces/${encodeURIComponent(seedData.workspaceId)}/integrations/azure-devops`,
  );

  await testPage.getByTestId("azure-devops-organization").fill("https://dev.azure.com/acme");
  await testPage.getByRole("button", { name: "How to create a personal access token" }).click();
  const createTokenLink = testPage.getByTestId("azure-devops-pat-help").locator(":scope > a");
  await expect(createTokenLink).toBeVisible();
  await expect(createTokenLink).toHaveAttribute(
    "href",
    "https://dev.azure.com/acme/_usersSettings/tokens",
  );
  await expect(testPage.getByTestId("azure-devops-pat-help")).toContainText(
    "Leave all other scopes unchecked",
  );
  const quickActions = testPage
    .getByRole("heading", { name: "Quick actions" })
    .locator("xpath=ancestor::section");
  await expect(quickActions.getByRole("tab", { name: "Pull requests" })).toBeVisible();
  await expect(quickActions.getByRole("tab", { name: "Work items" })).toBeVisible();
  const defaultQueries = testPage
    .getByRole("heading", { name: "Default queries" })
    .locator("xpath=ancestor::section");
  await expect(defaultQueries.getByRole("tab", { name: "Pull requests" })).toBeVisible();
  await expect(defaultQueries.getByRole("tab", { name: "Work items" })).toBeVisible();
  const resetBox = await quickActions.getByRole("button", { name: "Reset" }).boundingBox();
  expect(resetBox).not.toBeNull();
  expect(resetBox!.height).toBeGreaterThanOrEqual(44);

  const viewportFits = await testPage.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth,
  );
  expect(viewportFits).toBe(true);
});

test("mobile board opens a focused column editor without horizontal overflow", async ({
  apiClient,
  seedData,
  testPage,
  prCapture,
}) => {
  await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
  await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
    organizationUrl: "https://dev.azure.com/acme",
    pat: "azure-test-pat",
  });
  await testPage.goto("/azure-devops");

  await expect(testPage.getByTestId("azure-devops-board")).toBeVisible();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();
  await prCapture.screenshot("board-mobile", {
    caption: "Azure DevOps focused mobile board column",
  });
  await testPage.getByTestId("azure-board-card-101").click();
  await expect(testPage.getByTestId("azure-work-item-detail")).toBeVisible();
  await expect(testPage.getByTestId("azure-work-item-detail-description")).toContainText(
    "Rotate the credentials safely.",
  );
  await expect(testPage.getByTestId("azure-work-item-detail-comments")).toContainText(
    "Discussion from Azure",
  );
  await testPage.getByTestId("azure-work-item-assign-current-user").click();
  await testPage.getByTestId("azure-work-item-column").click();
  await testPage.getByRole("option", { name: "Active" }).click();
  await testPage.getByTestId("azure-work-item-column-done").click();
  await testPage.getByRole("option", { name: "Done" }).click();
  await testPage.getByRole("button", { name: "Move" }).click();
  await testPage.getByTestId("azure-work-item-detail-close").click();
  await testPage.getByTestId("azure-board-column-picker").click();
  await testPage.getByTestId("azure-board-column-option-active").click();
  await expect(testPage.getByText("Handle token rotation")).toBeVisible();

  await testPage.getByTestId("azure-devops-work-items-mode").click();
  await testPage.getByTestId("azure-devops-mobile-filter-button").click();
  await testPage.getByTestId("azure-devops-search-button-mobile").click();
  await testPage.getByTestId("azure-work-item-row-101").click();
  await expect(testPage.getByTestId("azure-work-item-quick-actions")).toBeVisible();
  await testPage.getByRole("button", { name: "Implement" }).click();
  const taskDialog = testPage.getByTestId("create-task-dialog");
  await expect(taskDialog).toBeVisible();
  await expect(taskDialog.getByTestId("task-title-input")).toHaveValue(
    "Implement: Handle token rotation",
  );
  await testPage.getByRole("button", { name: "Create only" }).click();
  await expect(testPage).toHaveURL(/\/tasks\//);
  await testPage.goto("/azure-devops");
  await testPage.getByTestId("azure-devops-work-items-mode").click();
  await testPage.getByTestId("azure-devops-mobile-filter-button").click();
  await testPage.getByTestId("azure-devops-search-button-mobile").click();
  await testPage.getByTestId("azure-devops-pull-requests-mode").click();
  await testPage.getByTestId("azure-devops-mobile-filter-button").click();
  await testPage.getByTestId("azure-devops-search-button-mobile").click();
  await expect(testPage.getByText("Rotate integration credentials")).toBeVisible();
  await testPage.getByRole("button", { name: "Feedback" }).click();
  await expect(testPage.getByTestId("azure-devops-feedback-detail")).toContainText(
    "Grace Reviewer",
  );

  const viewportFits = await testPage.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth,
  );
  expect(viewportFits).toBe(true);

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
  await workItemWatch.getByRole("button", { name: "Edit" }).click();
  await expect(testPage.getByTestId("azure-watch-editor-close")).toBeVisible();
  await testPage.getByTestId("azure-watch-editor-close").click();
  await workItemWatch.getByRole("button", { name: "Run now" }).click();
  await expect(testPage.getByText(/\d+ new match/)).toBeVisible();
  await workItemWatch.getByRole("button", { name: "Reset" }).click();
  const resetDialog = testPage.getByTestId("reset-watch-dialog");
  await expect(resetDialog).toBeVisible();
  await resetDialog.getByTestId("reset-watch-dialog-confirm").tap();
  await expect(testPage.getByText("Watch reset.")).toBeVisible();
});
