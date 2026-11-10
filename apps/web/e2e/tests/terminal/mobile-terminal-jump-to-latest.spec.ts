import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  focusTerminalForTyping,
  switchToTerminalPanel,
  waitForShellReady,
} from "./mobile-terminal-helpers";
import { OUTPUT_COMMAND, verifyJump } from "./terminal-jump-helpers";
import { attachShellInputCapture } from "../../helpers/ws-capture";

test("phone task shell returns to latest output", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(120_000);
  const { frames } = attachShellInputCapture(testPage);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Phone jump shell",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle();
  await switchToTerminalPanel(testPage);
  await waitForShellReady(testPage);
  await focusTerminalForTyping(testPage);
  await testPage.keyboard.type(OUTPUT_COMMAND, { delay: 5 });
  await testPage.keyboard.press("Enter");
  await verifyJump(
    testPage,
    testPage.locator('[data-testid^="mobile-terminal-slot-"][data-state="active"]'),
    true,
    () => prCapture.screenshot("phone", { caption: "Phone task terminal jump to latest output" }),
    frames,
  );
});
