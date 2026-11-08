import { expect, test } from "../../fixtures/test-base";
import type { Route } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import { PrAssetCapture } from "../../helpers/pr-asset-capture";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

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

// @covers AC-UI-COMPOSER-ACTION-WRAP-001.7
test("composer PR links keep phone targets reachable and desktop sizing compact", async ({
  apiClient,
  seedData,
  testPage,
  desktopTestPage,
}, testInfo) => {
  const repository = await apiClient.getRepository(seedData.repositoryId);
  const capture = new PrAssetCapture(testPage, testInfo.file, { captureKey: testInfo.title });
  try {
    await apiClient.mockAzureDevOpsSeed(MOCK_STATE);
    await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
      organizationUrl: "https://dev.azure.com/acme",
      pat: "azure-test-pat",
    });
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "azure_devops",
      provider_repo_id: "azure-repo-1",
      provider_host: "dev.azure.com/acme",
      provider_owner: "project-1",
      provider_name: "azure-repo-1",
    });
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Azure composer target",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.associateAzureDevOpsTaskPR(
      seedData.workspaceId,
      task.id,
      seedData.repositoryId,
      42,
    );
    const url = MOCK_STATE.pullRequests[0].webUrl;
    for (const page of [testPage, desktopTestPage]) {
      await page.setViewportSize({ width: 320, height: 851 });
      await page.goto(`/t/${task.id}`);
      const session = new SessionPage(page);
      await session.waitForLoad();
      await expect(session.activeChat()).toContainText(
        "This is a simple mock response for e2e testing.",
      );
      const chip = session.chatStatusBar().getByRole("link", { name: /PR 42/ });
      await expect(chip).toBeVisible();
      await expect
        .poll(async () => {
          const box = await chip.boundingBox();
          return box ? Math.min(box.width, box.height) : 0;
        })
        .toBeGreaterThanOrEqual(44);
      expect(
        await chip.evaluate((link) => {
          const box = link.getBoundingClientRect();
          return link.contains(
            document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
          );
        }),
      ).toBe(true);
      await page
        .context()
        .route(url, (route) => route.fulfill({ body: "Mock Azure pull request" }));
      const opened = page.waitForEvent("popup");
      if (page === testPage) await chip.tap();
      else await chip.click();
      const popup = await opened;
      await expect(popup).toHaveURL(url);
      await popup.close();
      await page.mouse.move(0, 0);
      await page.keyboard.press("Escape");
      await expect(page.getByRole("tooltip")).toBeHidden();
      await waitForFiniteAnimations(session.chatStatusBar());
      if (page === testPage) {
        await capture.screenshot("mobile-azure-composer-target", {
          caption: "Azure DevOps composer PR link keeps a 44px target on a 320px phone",
        });
      } else {
        await page.setViewportSize({ width: 1280, height: 900 });
        await expect.poll(async () => (await chip.boundingBox())?.height ?? 100).toBeLessThan(44);
        await capture.screenshot("desktop-azure-composer-target", {
          page,
          caption: "Azure DevOps composer PR link retains compact desktop sizing",
        });
      }
    }
    capture.flush();
  } finally {
    await apiClient.updateRepository(seedData.repositoryId, repository);
  }
});

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
  const savedViewsSeed = await testPage.request.put(
    `/api/v1/azure-devops/views?workspace_id=${encodeURIComponent(seedData.workspaceId)}`,
    {
      data: {
        views: [
          {
            id: "mobile-security-queue",
            kind: "work_item",
            label: "Mobile security queue",
            projectId: "project-1",
            wiql: "SELECT [System.Id] FROM WorkItems",
            createdAt: "2026-09-07T00:00:00Z",
          },
        ],
      },
    },
  );
  expect(savedViewsSeed.ok()).toBe(true);
  await testPage.goto("/azure-devops");

  await expect(testPage.getByTestId("azure-devops-board")).toBeVisible();
  const savedViewsMenu = testPage.getByTestId("azure-devops-saved-views-menu");
  await savedViewsMenu.tap();
  const deleteSavedView = testPage.getByRole("menuitem", {
    name: "Delete Mobile security queue saved query",
  });
  await expect
    .poll(async () => (await deleteSavedView.boundingBox())?.height ?? 0)
    .toBeGreaterThanOrEqual(44);
  await expect
    .poll(async () => (await deleteSavedView.boundingBox())?.width ?? 0)
    .toBeGreaterThanOrEqual(44);
  await deleteSavedView.tap();
  const deleteConfirmation = testPage.getByTestId("saved-task-view-delete-confirmation");
  await expect(deleteConfirmation).toHaveAccessibleName("Delete Mobile security queue?");
  await expect(testPage.locator('[role="dialog"]:visible')).toHaveCount(1);
  await expect(testPage.getByRole("menu")).toHaveCount(0);
  for (const action of await deleteConfirmation.getByRole("button").all()) {
    const box = await action.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
  }
  await deleteConfirmation.getByRole("button", { name: "Cancel" }).tap();
  await expect(savedViewsMenu).toBeFocused();
  await savedViewsMenu.tap();
  await expect(deleteSavedView).toBeVisible();

  await deleteSavedView.tap();
  const deleteSavedViewResponse = testPage.waitForResponse(
    (response) =>
      response.ok() &&
      response.request().method() === "PUT" &&
      response.url().includes("/api/v1/azure-devops/views"),
  );
  await testPage
    .getByTestId("saved-task-view-delete-confirmation")
    .getByRole("button", { name: "Delete Mobile security queue" })
    .tap();
  await deleteSavedViewResponse;
  await expect(deleteSavedView).toHaveCount(0);
  await savedViewsMenu.tap();
  await expect(deleteSavedView).toHaveCount(0);
  await testPage.keyboard.press("Escape");
  await expect(testPage.getByRole("menu")).toBeHidden();
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
  const assign = testPage.getByTestId("azure-work-item-assign-current-user");
  await assign.click();
  await expect(assign).toBeEnabled();
  await testPage.getByTestId("azure-work-item-column").click();
  await testPage.getByRole("option", { name: "Active" }).click();
  await testPage.getByTestId("azure-work-item-column-done").click();
  await testPage.getByRole("option", { name: "Done" }).click();
  let releaseMove!: () => void;
  let observeMove!: () => void;
  const heldMove = new Promise<void>((resolve) => {
    releaseMove = resolve;
  });
  const moveReached = new Promise<void>((resolve) => {
    observeMove = resolve;
  });
  const movePattern = "**/azure-devops/boards/*/work-items/101?**";
  const holdMove = async (route: Route) => {
    if (route.request().method() !== "PATCH") {
      await route.fallback();
      return;
    }
    const response = await route.fetch();
    observeMove();
    await heldMove;
    await route.fulfill({ response });
  };
  await testPage.route(movePattern, holdMove);
  const moveResponse = testPage.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      new URL(response.url()).pathname.endsWith("/work-items/101"),
  );
  try {
    await testPage.getByRole("button", { name: "Move" }).click();
    await moveReached;
    await testPage.getByTestId("azure-work-item-detail-close").click();
    await expect(testPage.getByTestId("azure-work-item-detail")).toHaveCount(0);
    releaseMove();
    expect((await moveResponse).ok()).toBe(true);
    await testPage.getByTestId("azure-board-column-picker").click();
    await testPage.getByTestId("azure-board-column-option-active").click();
    await expect(testPage.getByText("Handle token rotation")).toBeVisible();
    await expect(testPage.getByTestId("azure-work-item-detail")).toHaveCount(0);
  } finally {
    releaseMove();
    await testPage.unroute(movePattern, holdMove);
  }
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
  await expect(testPage).toHaveURL(/\/(?:t|tasks)\//);
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
