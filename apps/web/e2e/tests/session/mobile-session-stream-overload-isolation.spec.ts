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
    const layout = testPage.locator("[data-testid='mobile-task-layout']:visible");
    await layout.waitFor({ state: "visible", timeout: 30_000 });

    const launched = await apiClient.launchSession({
      task_id: noisyTask.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: 'e2e:message("noisy-ready")',
    });
    const noisySessionId = launched.session_id;

    await session.waitForLoad();
    const noisyPage = await testPage.context().newPage();
    const noisyCapture = attachGatewayTrafficCapture(noisyPage);
    await noisyPage.goto(`/t/${noisyTask.id}`);
    const noisySession = new SessionPage(noisyPage);
    await noisySession.waitForLoad();
    const noisyLayout = noisyPage.locator("[data-testid='mobile-task-layout']:visible");
    const noisyPill = noisyLayout.getByTestId("mobile-sessions-pill");
    await expect(noisyPill).toBeVisible({ timeout: 30_000 });
    await noisyPill.tap();
    const noisySheet = noisyPage.getByRole("dialog", { name: "Sessions" });
    await expect(noisySheet).toBeVisible({ timeout: 30_000 });
    const noisyRow = noisySheet.getByTestId(`mobile-session-row-${noisySessionId}`);
    await expect(noisyRow).toBeVisible({ timeout: 30_000 });
    await noisyRow.tap();
    await noisySession.waitForLoad();
    await noisySession.waitForChatIdle({ timeout: 60_000 });
    await expect
      .poll(
        () =>
          noisyCapture.frames.some(
            (frame) =>
              frame.direction === "sent" &&
              frame.action === "session.subscribe" &&
              frame.sessionId === noisySessionId,
          ),
        { message: "noisy mobile page must subscribe before the burst", timeout: 10_000 },
      )
      .toBe(true);

    const pill = layout.getByTestId("mobile-sessions-pill");
    await expect(pill).toBeVisible({ timeout: 30_000 });
    await pill.tap();
    const quietSheet = testPage.getByRole("dialog", { name: "Sessions" });
    await expect(quietSheet).toBeVisible({ timeout: 30_000 });
    const quietRow = quietSheet.getByTestId(`mobile-session-row-${quietSession.session_id}`);
    await expect(quietRow).toBeVisible({ timeout: 30_000 });
    await quietRow.tap();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    await expect
      .poll(
        () =>
          capture.frames.some(
            (frame) =>
              frame.direction === "sent" &&
              frame.action === "session.subscribe" &&
              frame.sessionId === quietSession.session_id,
          ),
        { message: "quiet mobile page must subscribe before the burst", timeout: 10_000 },
      )
      .toBe(true);

    noisyCapture.frames.length = 0;
    capture.frames.length = 0;
    await noisySession.sendMessageViaButton(reasoningBurstPrompt());
    await session.sendMessageViaButton("mobile-quiet-followup");
    await session.expectChatResponseVisible("mobile-quiet-followup", 0, { timeout: 60_000 });

    await waitForExactReasoningBurst(apiClient, noisySessionId);
    const noisyFrames = noisyReceivedFrames(noisyCapture.frames, noisySessionId);
    const quietSessionNoisyFrames = noisyReceivedFrames(capture.frames, noisySessionId);
    expect(noisyFrames.length).toBeGreaterThan(0);
    expect(noisyFrames.length).toBeLessThan(REASONING_BURST_COUNT);
    expect(
      quietSessionNoisyFrames,
      "quiet mobile view must not receive noisy-session message frames",
    ).toHaveLength(0);
    await assertNoHorizontalOverflow(testPage, "mobile session stream overload surface");

    const evidence = {
      sourceChunks: REASONING_BURST_COUNT,
      gatewayReceivedMessageFrames: noisyFrames.length,
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
