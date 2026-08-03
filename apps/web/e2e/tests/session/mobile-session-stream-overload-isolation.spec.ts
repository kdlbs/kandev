import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  assertNoHorizontalOverflow,
  noisyReceivedFrames,
  reasoningBurstPrompt,
  REASONING_BURST_COUNT,
  waitForExactReasoningBurst,
} from "../../helpers/session-stream-overload";
import { attachGatewayTrafficCapture, summarizeGatewayTraffic } from "../../helpers/ws-traffic";

test.describe("mobile: session stream overload isolation", () => {
  test("switches through the native session sheet while a sibling streams", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const capture = attachGatewayTrafficCapture(testPage);
    const noisyTask = await apiClient.createTask(
      seedData.workspaceId,
      "Mobile noisy reasoning task",
      {
        agent_profile_id: seedData.agentProfileId,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    const quietSession = await apiClient.seedTaskSession(noisyTask.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
      repositoryId: seedData.repositoryId,
      sessionId: `mobile-quiet-${noisyTask.id}`,
    });

    await testPage.goto(`/t/${noisyTask.id}`);
    const session = new SessionPage(testPage);
    await testPage
      .locator("[data-testid='mobile-task-layout']:visible")
      .waitFor({ state: "visible", timeout: 30_000 });

    const launched = await apiClient.launchSession({
      task_id: noisyTask.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: reasoningBurstPrompt(),
    });
    const noisySessionId = launched.session_id;

    await session.waitForLoad();
    const noisyPage = await testPage.context().newPage();
    const noisyCapture = attachGatewayTrafficCapture(noisyPage);
    await noisyPage.goto(`/t/${noisyTask.id}`);
    const noisySession = new SessionPage(noisyPage);
    await noisySession.waitForLoad();

    const pill = testPage.getByTestId("mobile-sessions-pill");
    await expect(pill).toBeVisible({ timeout: 30_000 });
    await pill.tap();
    const quietRow = testPage.getByTestId(`mobile-session-row-${quietSession.session_id}`);
    await expect(quietRow).toBeVisible({ timeout: 30_000 });
    await quietRow.tap();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await session.sendMessageViaButton("mobile-quiet-followup");
    await session.expectChatResponseVisible("mobile-quiet-followup", 0, { timeout: 60_000 });

    await waitForExactReasoningBurst(apiClient, noisySessionId);
    const noisyFrames = noisyReceivedFrames(noisyCapture.frames, noisySessionId);
    const quietSessionNoisyFrames = noisyReceivedFrames(capture.frames, noisySessionId);
    expect(noisyFrames.length).toBeGreaterThan(0);
    expect(noisyFrames.length).toBeLessThan(REASONING_BURST_COUNT);
    expect(
      quietSessionNoisyFrames,
      "quiet mobile view must not receive noisy-session updates",
    ).toHaveLength(0);
    await assertNoHorizontalOverflow(testPage, "mobile session stream overload surface");

    const evidence = {
      sourceChunks: REASONING_BURST_COUNT,
      gatewayReceivedUpdatedFrames: noisyFrames.length,
      noisySessionId,
      quietSessionId: quietSession.session_id,
      quietResponse: "mobile-quiet-followup",
      gateway: summarizeGatewayTraffic(capture.frames),
      noisyGateway: summarizeGatewayTraffic(noisyCapture.frames),
    };
    await test.info().attach("mobile-session-stream-overload-evidence.json", {
      body: JSON.stringify(evidence, null, 2),
      contentType: "application/json",
    });
    test.info().annotations.push({
      type: "stream-overload-evidence",
      description: JSON.stringify(evidence),
    });
    await noisyPage.close();
  });
});
