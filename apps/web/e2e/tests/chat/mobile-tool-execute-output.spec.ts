import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

async function expectInsideViewport(
  testPage: import("@playwright/test").Page,
  locator: import("@playwright/test").Locator,
) {
  const viewportWidth = testPage.viewportSize()?.width ?? 0;
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(viewportWidth + 1);
}

test.describe("mobile: shell command output", () => {
  test("keeps a long command and lazy output inside the viewport", async ({
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
    const command = `printf ${"mobile-command-segment-".repeat(28)}`;
    const outputPrefix = "latest output";
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
              stdout: `${outputPrefix} ${"unbroken-output-".repeat(80)}`,
              truncated: true,
            },
          },
        },
      },
    });
    const shellOutputRequests: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().endsWith("/shell-output")) shellOutputRequests.push(request.url());
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const chat = session.activeChat();
    const commandRow = chat.getByTestId("tool-execute-command").filter({ hasText: command });
    const disclosure = chat.getByRole("button", { name: "Show command output" });

    await expect(commandRow).toBeVisible({ timeout: 15_000 });
    await expect(commandRow).toHaveText(command);
    await expect(chat.getByTestId("tool-execute-output")).toHaveCount(0);
    await expect(chat.getByText(outputPrefix)).toHaveCount(0);
    expect(shellOutputRequests).toHaveLength(0);
    expect(await commandRow.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    await expectInsideViewport(testPage, commandRow);
    await expectInsideViewport(testPage, disclosure);

    const responsePromise = testPage.waitForResponse(
      (response) => response.url().endsWith("/shell-output") && response.status() === 200,
    );
    await disclosure.click();
    await responsePromise;

    const output = chat.getByTestId("tool-execute-output");
    const exitStatus = chat.getByText("Exit code 9");
    await expect(output).toContainText(outputPrefix);
    await expect(chat.getByText("Output truncated")).toBeVisible();
    await expect(exitStatus).toBeVisible();
    expect(shellOutputRequests).toHaveLength(1);

    await expectInsideViewport(testPage, output);
    await expectInsideViewport(testPage, exitStatus);
    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  });
});
