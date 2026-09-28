import { test, expect } from "../../fixtures/test-base";

const repositoryOwner = "e2e-topbar-owner";
const repositoryName = "agent-orchestrator";
const repositoryBrowserUrl = `https://github.com/${repositoryOwner}/${repositoryName}`;
const taskTitle = "Explain agent connections";

test.describe("Task topbar remote repository", () => {
  // @covers AC-UI-REMOTE-REPO-TOPBAR-001.1, AC-UI-REMOTE-REPO-TOPBAR-001.2
  test("opens the repository in a new tab and keeps the task title usable", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, taskTitle, {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repositories: [
        {
          remote_url: `${repositoryBrowserUrl}.git`,
          provider: "github",
          provider_owner: repositoryOwner,
          provider_name: repositoryName,
        },
      ],
    });

    await testPage.goto(`/t/${task.id}`);

    const topbar = testPage.getByTestId("task-topbar");
    const repositoryLink = topbar.getByRole("link", {
      name: `GitHub repository ${repositoryOwner}/${repositoryName}`,
    });
    await expect(repositoryLink).toHaveAttribute("href", repositoryBrowserUrl);
    await expect(repositoryLink).toHaveAttribute("target", "_blank");
    await expect(repositoryLink).toHaveAttribute("rel", "noopener noreferrer");
    await expect(repositoryLink).toContainText(repositoryName);
    await prCapture.screenshot("task-topbar-remote-repository-desktop", {
      caption: "Desktop task topbar with its remote repository link before the task title.",
    });

    const title = topbar.getByTestId("task-topbar-title");
    await expect(title).toHaveText(taskTitle);
    await title.dblclick();
    const renameInput = testPage.getByTestId("task-title-rename-input");
    await expect(renameInput).toBeVisible();
    await renameInput.press("Escape");

    await testPage
      .context()
      .route("https://github.com/**", (route) =>
        route.fulfill({ status: 200, contentType: "text/html", body: "repository" }),
      );
    const popupPromise = testPage.waitForEvent("popup");
    await repositoryLink.click();
    const popup = await popupPromise;
    await expect.poll(() => popup.url()).toBe(repositoryBrowserUrl);
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
  });

  test("measures the repository crumb within its visible width cap", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const longRepositoryName =
      "agent-orchestrator-with-a-deliberately-long-name-for-breadcrumb-measurement";
    const longRepositoryUrl = `https://github.com/${repositoryOwner}/${longRepositoryName}`;
    const task = await apiClient.createTask(
      seedData.workspaceId,
      "Measure a long repository crumb",
      {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [
          {
            remote_url: `${longRepositoryUrl}.git`,
            provider: "github",
            provider_owner: repositoryOwner,
            provider_name: longRepositoryName,
          },
        ],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const repositoryLink = testPage.getByRole("link", {
      name: `GitHub repository ${repositoryOwner}/${longRepositoryName}`,
    });
    await expect(repositoryLink).toBeVisible();
    const visibleWidth = await repositoryLink.evaluate(
      (element) => element.getBoundingClientRect().width,
    );
    const ghostLabel = testPage.locator(`span[data-label="${longRepositoryName}"]`).first();
    const measuredWidth = await ghostLabel.evaluate(
      (element) => element.parentElement?.getBoundingClientRect().width ?? 0,
    );

    expect(visibleWidth).toBeLessThanOrEqual(160);
    expect(measuredWidth).toBeCloseTo(visibleWidth, 0);
  });
});
