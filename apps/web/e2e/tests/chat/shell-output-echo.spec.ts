import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Regression for a transcript bug: some providers capture shell output via a
// real terminal session whose input echo is concatenated directly onto the
// real output, so the persisted stdout opens with a verbatim repeat of the
// command the chat already renders above the Output disclosure. The ACP
// shell-output normalizer strips that leading echo (unit-tested in
// shell_output_test.go); this file drives the mock-agent through the real
// ACP pipeline and asserts the chat UI never shows the command twice.
test.describe("shell command output echo stripping", () => {
  test("does not repeat the command inside the expanded Output disclosure", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const command = "cat file.txt";
    const echoedOutput = `${command}=== marker ===\n`;

    const script = [
      `e2e:shell_result("${command}", "${echoedOutput.replace(/\n/g, "\\n")}")`,
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Shell echo regression",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const chat = session.activeChat();
    const commandRow = chat.getByTestId("tool-execute-command").filter({ hasText: command });
    await expect(commandRow).toBeVisible();
    await expect(commandRow).toHaveText(command);

    const disclosure = chat.getByRole("button", { name: "Show command output" });
    const responsePromise = testPage.waitForResponse(
      (response) => response.url().endsWith("/shell-output") && response.status() === 200,
    );
    await disclosure.click();
    await responsePromise;

    const outputRegion = chat.getByTestId("tool-execute-output");
    await expect(outputRegion).toContainText("=== marker ===");
    // Critical: the echoed command must not also appear inside the Output
    // disclosure - it's already shown once in the command row above.
    await expect(outputRegion).not.toContainText(command);
  });
});
