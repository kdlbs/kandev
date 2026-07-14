import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("mobile: shell command output", () => {
  test("keeps truncated output and exit status inside the viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Mobile Command Output", {
      description: "seeded mobile shell command output",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "IDLE",
      agentProfileId: seedData.agentProfileId,
    });
    const command = "printf mobile-command-output";
    await apiClient.seedSessionMessage(sessionId, {
      type: "tool_execute",
      content: command,
      metadata: {
        status: "error",
        tool_call_id: "tool-mobile-output",
        normalized: {
          shell_exec: {
            command,
            work_dir: "/workspace/a/very/long/path/that/must/not/expand/the/page",
            output: {
              exit_code: 9,
              stdout: `latest output ${"unbroken-output-".repeat(80)}`,
              truncated: true,
            },
          },
        },
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chat = session.activeChat();
    const commandRow = chat.getByTestId("tool-execute-command").filter({ hasText: command });
    await expect(commandRow).toBeVisible({ timeout: 15_000 });
    await commandRow.click();

    const output = chat.getByTestId("tool-execute-output");
    const details = chat.getByTestId("tool-execute-result-details");
    await expect(output).toContainText("latest output");
    await expect(details).toContainText("Output truncated");
    await expect(details).toContainText("Exit code 9");

    const viewportWidth = testPage.viewportSize()?.width ?? 0;
    const outputBox = await output.boundingBox();
    expect(outputBox).not.toBeNull();
    expect(outputBox!.x).toBeGreaterThanOrEqual(0);
    expect(outputBox!.x + outputBox!.width).toBeLessThanOrEqual(viewportWidth + 1);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
