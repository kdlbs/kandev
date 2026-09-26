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

test.describe("Session stream overload isolation", () => {
  test("keeps a quiet session usable while another session streams", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const quietCapture = attachGatewayTrafficCapture(testPage);

    const noisyTask = await apiClient.createTask(
      seedData.workspaceId,
      "Noisy reasoning isolation task",
      {
        agent_profile_id: seedData.agentProfileId,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const preparedNoisy = await apiClient.launchSession({
      task_id: noisyTask.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "",
      intent: "prepare",
      launch_workspace: true,
    });
    const noisySessionId = preparedNoisy.session_id;

    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(noisyTask.id))?.status ?? null, {
        timeout: 60_000,
        message: "prepared noisy task environment did not become ready",
      })
      .toBe("ready");

    // Keep a real subscribed browser open for the noisy session while the
    // primary page navigates to the quiet task. This exercises the same
    // per-client isolation boundary that failed during the incident. The
    // session is prepared first so a fast mock run cannot finish before the
    // browser has registered its subscription.
    const noisyPage = await testPage.context().newPage();
    const noisyCapture = attachGatewayTrafficCapture(noisyPage);
    await noisyPage.goto(`/t/${noisyTask.id}`);
    const noisySession = new SessionPage(noisyPage);
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
        { timeout: 10_000, message: "noisy page must subscribe before the burst" },
      )
      .toBe(true);
    await expect
      .poll(
        () =>
          noisyCapture.frames.some(
            (frame) =>
              frame.direction === "received" &&
              frame.action === "session.subscribe" &&
              frame.sessionId === noisySessionId,
          ),
        { timeout: 30_000, message: "noisy page did not acknowledge its session subscription" },
      )
      .toBe(true);

    const quietTask = await apiClient.createTask(
      seedData.workspaceId,
      "Quiet session isolation task",
      {
        agent_profile_id: seedData.agentProfileId,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    const preparedQuiet = await apiClient.launchSession({
      task_id: quietTask.id,
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
      workflow_step_id: seedData.startStepId,
      prompt: "",
      intent: "prepare",
      launch_workspace: true,
    });
    const quietSessionId = preparedQuiet.session_id;

    await expect
      .poll(async () => (await apiClient.getTaskEnvironment(quietTask.id))?.status ?? null, {
        timeout: 60_000,
        message: "prepared quiet task environment did not become ready",
      })
      .toBe("ready");

    await testPage.goto(`/t/${quietTask.id}`);
    const quietSession = new SessionPage(testPage);
    await quietSession.waitForLoad();
    await quietSession.waitForChatIdle({ timeout: 60_000 });
    await expect
      .poll(
        () =>
          quietCapture.frames.some(
            (frame) =>
              frame.direction === "sent" &&
              frame.action === "session.subscribe" &&
              frame.sessionId === quietSessionId,
          ),
        { timeout: 10_000, message: "quiet page must subscribe before the burst" },
      )
      .toBe(true);
    await expect
      .poll(
        () =>
          quietCapture.frames.some(
            (frame) =>
              frame.direction === "received" &&
              frame.action === "session.subscribe" &&
              frame.sessionId === quietSessionId,
          ),
        { timeout: 30_000, message: "quiet page did not acknowledge its session subscription" },
      )
      .toBe(true);

    noisyCapture.frames.length = 0;
    quietCapture.frames.length = 0;
    await apiClient.launchSession({
      task_id: noisyTask.id,
      session_id: noisySessionId,
      agent_profile_id: seedData.agentProfileId,
      prompt: reasoningBurstPrompt(),
      intent: "start_created",
    });

    await quietSession.sendMessageViaButton("quiet-session-followup");
    await quietSession.expectChatResponseVisible("quiet-session-followup", 0, { timeout: 60_000 });
    await assertNoHorizontalOverflow(testPage, "session stream overload desktop surface");

    const exact = await waitForExactReasoningBurst(apiClient, noisySessionId);
    const noisyFrames = noisyReceivedFrames(noisyCapture.frames, noisySessionId);
    const quietSessionNoisyFrames = noisyReceivedFrames(quietCapture.frames, noisySessionId);
    expect(
      noisyFrames.length,
      "noisy session must produce observable message frames",
    ).toBeGreaterThan(0);
    expect(noisyFrames.length, "gateway delivery must be below source chunk count").toBeLessThan(
      REASONING_BURST_COUNT,
    );
    expect(
      quietSessionNoisyFrames,
      "quiet page must not receive noisy-session message frames",
    ).toHaveLength(0);

    const evidence = {
      sourceChunks: exact.sourceChunks,
      reasoningBytes: exact.reasoningBytes,
      gatewayReceivedMessageFrames: noisyFrames.length,
      quietSessionResponse: "quiet-session-followup",
      quietGateway: summarizeGatewayTraffic(quietCapture.frames),
      noisyGateway: summarizeGatewayTraffic(noisyCapture.frames),
    };
    await test.info().attach("session-stream-overload-evidence.json", {
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
