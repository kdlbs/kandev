import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Regression for a transcript bug: some providers capture shell output via a
// real terminal session whose input echo - rendered behind a "$ " shell
// prompt - is concatenated directly onto the real output, so the persisted
// stdout opens with a verbatim repeat of the command the chat already
// renders above the Output disclosure. The ACP shell-output normalizer
// strips that leading echo (unit-tested in shell_output_test.go); this file
// drives the mock-agent through the real ACP pipeline and asserts the chat
// UI never shows the command twice.
test.describe("shell command output echo stripping", () => {
  test("does not repeat the command inside the expanded Output disclosure", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const command = "cat file.txt";
    const echoedOutput = `$ ${command}=== marker ===\n`;

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

  test("does not repeat a workDir-resolved absolute-path echo inside the expanded Output disclosure", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Some providers resolve a relative file-path argument in the command
    // to an absolute (cwd-joined) one before actually invoking the real
    // terminal, while the reported "command" - the same text the chat
    // header renders - keeps the original relative form. The captured echo
    // line then no longer matches the command byte-for-byte.
    const command = "cat notes.txt";
    const cwd = "/workspace/example-repo";
    const resolvedCommand = `cat ${cwd}/notes.txt`;
    const echoedOutput = `$ ${resolvedCommand}\nhello\n`;

    const script = [
      `e2e:shell_result("${command}", "${echoedOutput.replace(/\n/g, "\\n")}", "${cwd}")`,
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Shell echo workDir regression",
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
    // The header keeps rendering the original relative command, unaffected
    // by the provider's internal absolute-path resolution.
    await expect(commandRow).toHaveText(command);

    const disclosure = chat.getByRole("button", { name: "Show command output" });
    const responsePromise = testPage.waitForResponse(
      (response) => response.url().endsWith("/shell-output") && response.status() === 200,
    );
    await disclosure.click();
    await responsePromise;

    const outputRegion = chat.getByTestId("tool-execute-output");
    await expect(outputRegion).toContainText("hello");
    // Critical: neither the workDir-prefixed absolute path nor a repeat of
    // the resolved command may leak into the Output disclosure.
    await expect(outputRegion).not.toContainText(cwd);
    await expect(outputRegion).not.toContainText(resolvedCommand);
  });

  test("does not repeat a multi-line workDir-resolved echo inside the expanded Output disclosure", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    // Regression for a report that the workDir-resolved-path fix above
    // still leaked the echo: a command that itself carries an embedded
    // newline (a multi-line script or commit message passed as a single
    // tool-call argument) is echoed by the real terminal across as many
    // lines as it spans, not just the first.
    const command = "echo start\ncat notes.txt";
    const cwd = "/workspace/example-repo";
    const resolvedCommand = `echo start\ncat ${cwd}/notes.txt`;
    const echoedOutput = `$ ${resolvedCommand}\nstart\nhello\n`;

    const script = [
      `e2e:shell_result("${command.replace(/\n/g, "\\n")}", "${echoedOutput.replace(/\n/g, "\\n")}", "${cwd}")`,
      'e2e:message("done")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Shell echo multiline workDir regression",
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
    const commandRow = chat.getByTestId("tool-execute-command").filter({ hasText: "echo start" });
    await expect(commandRow).toBeVisible();
    await expect(commandRow).toHaveText(command);

    const disclosure = chat.getByRole("button", { name: "Show command output" });
    const responsePromise = testPage.waitForResponse(
      (response) => response.url().endsWith("/shell-output") && response.status() === 200,
    );
    await disclosure.click();
    await responsePromise;

    const outputRegion = chat.getByTestId("tool-execute-output");
    await expect(outputRegion).toContainText("start");
    await expect(outputRegion).toContainText("hello");
    // Critical: neither the workDir-prefixed absolute path nor a repeat of
    // the resolved multi-line command may leak into the Output disclosure.
    await expect(outputRegion).not.toContainText(cwd);
    await expect(outputRegion).not.toContainText("cat /workspace");

    // Visual evidence: the Output disclosure shows only "start\nhello",
    // never the "$ echo start" / "cat /workspace/..." echo lines.
    await testPage.screenshot({
      path: testInfo.outputPath("shell-output-multiline-workdir-echo-fixed.png"),
      fullPage: true,
    });
  });
});
