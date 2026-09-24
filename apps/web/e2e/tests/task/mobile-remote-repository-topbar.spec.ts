import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

const repositoryOwner = "e2e-phone-owner";
const repositoryName =
  "agent-orchestrator-with-a-deliberately-long-name-for-phone-truncation-check";
const repositoryBrowserUrl = `https://github.com/${repositoryOwner}/${repositoryName}`;
const taskTitle = "Explain agent connections on a phone";

test.describe("Mobile task topbar remote repository", () => {
  // @covers AC-UI-REMOTE-REPO-TOPBAR-001.1, AC-UI-REMOTE-REPO-TOPBAR-001.2,
  // AC-UI-REMOTE-REPO-TOPBAR-001.5
  test("keeps the repository link and task picker usable on a phone", async ({
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

    const repositoryLink = testPage.getByTestId("mobile-task-repository-link");
    const taskPicker = testPage.getByTestId("mobile-task-picker-trigger");
    await expect(repositoryLink).toHaveAttribute("href", repositoryBrowserUrl);
    await expect(repositoryLink).toHaveAttribute("target", "_blank");
    await expect(repositoryLink).toHaveAccessibleName(
      `GitHub repository ${repositoryOwner}/${repositoryName}`,
    );
    await expect(taskPicker).toContainText(taskTitle);

    const repositoryBox = await repositoryLink.boundingBox();
    const taskPickerBox = await taskPicker.boundingBox();
    expect(repositoryBox).not.toBeNull();
    expect(taskPickerBox).not.toBeNull();
    expect(repositoryBox!.width).toBeGreaterThanOrEqual(44);
    expect(repositoryBox!.height).toBeGreaterThanOrEqual(44);
    expect(taskPickerBox!.width).toBeGreaterThanOrEqual(44);
    await expect
      .poll(() =>
        testPage.getByTestId("mobile-task-repository-name").evaluate((node) => {
          const label = node as HTMLElement;
          return label.scrollWidth > label.clientWidth;
        }),
      )
      .toBe(true);
    await assertNoDocumentHorizontalOverflow(testPage, "remote repository task topbar");

    await prCapture.screenshot("task-topbar-remote-repository-mobile", {
      caption: "Phone task topbar with a truncated repository link and the task picker.",
    });
    await testPage
      .context()
      .route("https://github.com/**", (route) =>
        route.fulfill({ status: 200, contentType: "text/html", body: "repository" }),
      );
    const popupPromise = testPage.waitForEvent("popup");
    await repositoryLink.tap();
    const popup = await popupPromise;
    await expect.poll(() => popup.url()).toBe(repositoryBrowserUrl);
    await expect(taskPicker).toBeVisible();

    await taskPicker.tap();
    await expect(testPage.getByRole("dialog", { name: "Tasks" })).toBeVisible();
  });
});
