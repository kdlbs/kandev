// Routing: /t/{taskId}. The mobile- prefix selects the Pixel 5 project.
import { expect, test } from "../../fixtures/test-base";
import { openTaskSession } from "../../helpers/session";
import { expectTerminalTheme, readTerminalHostTheme } from "./terminal-test-helpers";
import { switchToTerminalPanel, waitForShellReady } from "./mobile-terminal-helpers";

test.describe("mobile adaptive terminal theme", () => {
  test("keeps the light-theme terminal readable on a phone", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile terminal theme",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await openTaskSession(testPage, task.id);
    await switchToTerminalPanel(testPage);
    await waitForShellReady(testPage);

    await expect(testPage.locator("html")).toHaveClass(/(^|\s)light(\s|$)/);
    const host = testPage
      .getByTestId("terminal-panel")
      .locator('[data-testid="terminal-xterm-host"]:visible');
    const theme = await readTerminalHostTheme(host);

    expectTerminalTheme(theme, "light", "the mobile xterm");
    await prCapture.screenshot("mobile-terminal-theme-light", {
      caption: "Pixel 5 task terminal with the readable light-theme palette",
    });
  });
});
