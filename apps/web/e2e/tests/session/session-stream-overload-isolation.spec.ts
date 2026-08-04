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

    const noisyTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Noisy reasoning isolation task",
      seedData.agentProfileId,
      {
        description: reasoningBurstPrompt(),
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!noisyTask.session_id) throw new Error("noisy task has no session");

    // Keep a real subscribed browser open for the noisy session while the
    // primary page navigates to the quiet task. This exercises the same
    // per-client isolation boundary that failed during the incident.
    const noisyPage = await testPage.context().newPage();
    const noisyCapture = attachGatewayTrafficCapture(noisyPage);
    await noisyPage.goto(`/t/${noisyTask.id}`);
    const noisySession = new SessionPage(noisyPage);
    await noisySession.waitForLoad();

    const quietTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Quiet session isolation task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!quietTask.session_id) throw new Error("quiet task has no session");

    await testPage.goto(`/t/${quietTask.id}`);
    const quietSession = new SessionPage(testPage);
    await quietSession.waitForLoad();
    await quietSession.waitForChatIdle({ timeout: 60_000 });
    await quietSession.sendMessageViaButton("quiet-session-followup");
    await quietSession.expectChatResponseVisible("quiet-session-followup", 0, { timeout: 60_000 });
    await assertNoHorizontalOverflow(testPage, "session stream overload desktop surface");

    const exact = await waitForExactReasoningBurst(apiClient, noisyTask.session_id);
    const noisyFrames = noisyReceivedFrames(noisyCapture.frames, noisyTask.session_id);
    const quietSessionNoisyFrames = noisyReceivedFrames(quietCapture.frames, noisyTask.session_id);
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
