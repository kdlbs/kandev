// AC-COORDINATOR-COPILOT-002.4, -003.9, -004.x, -005.x: the coordinator
// copilot launcher and popover (docs/plans/workspace-coordinator/task-06-copilot-wired.md).
import { type Locator, type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { waitForHttp } from "../../helpers/causal-waits";
import { captureGatewayRequests } from "../../helpers/archived-session-recovery";
import { linkToCoordinatorNeedsYou } from "../../../lib/coordinator/links";

const COORDINATOR_READ = /\/coordinators\/[^/]+$/;
const CONVERSATION_OPENED = /\/coordinators\/[^/]+\/conversation$/;

async function openCopilot(page: Page): Promise<Locator> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  await conversationOpened;
  const popover = page.getByTestId("coordinator-copilot-popover");
  await expect(popover).toBeVisible();
  return popover;
}

async function sendMessage(popover: Locator, text: string) {
  const editor = popover.getByTestId("chat-input-editor");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await expect(editor).toHaveAttribute("contenteditable", "true", { timeout: 15_000 });
  await editor.fill(text);
  await editor.press(`${modifier}+Enter`);
}

// Like openCopilot, but also surfaces the opened conversation's ids so a test
// can drive that session to a terminal state and later prove a Retry opened
// a different one.
async function openCopilotCapturingSession(
  page: Page,
): Promise<{ popover: Locator; sessionId: string; taskId: string }> {
  const coordinatorRead = waitForHttp(page, "GET", COORDINATOR_READ);
  const conversationOpened = waitForHttp(page, "POST", CONVERSATION_OPENED);
  const launcher = page.getByTestId("coordinator-copilot-launcher");
  await expect(launcher).toBeVisible({ timeout: 10_000 });
  await launcher.click();
  await coordinatorRead;
  const response = await conversationOpened;
  const body = (await response.json()) as { task_id: string; session_id: string };
  const popover = page.getByTestId("coordinator-copilot-popover");
  await expect(popover).toBeVisible();
  return { popover, sessionId: body.session_id, taskId: body.task_id };
}

test.describe("Coordinator copilot", () => {
  test("opens on the launcher, shows the empty-conversation intro and suggestion, and answers a question (AC .002.4, .004.3, .004.4, .004.8)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Copilot Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));

    const popover = await openCopilot(testPage);
    // Out of scope for this work order: no Expand, no Quick Chat tab.
    await expect(popover.getByTestId("quick-chat-tab")).toHaveCount(0);
    await expect(popover.getByRole("button", { name: "Open in Quick Chat" })).toHaveCount(0);
    // hideSessionSelectors widens the minimal toolbar to every session kind
    // (AC .004.3): no mode or model selector, not just Submit/Stop.
    await expect(popover.getByTestId("session-mode-selector")).toHaveCount(0);

    await expect(
      popover.getByText("Ask the coordinator about anything on this screen."),
    ).toBeVisible();
    const suggestion = popover.getByRole("button", { name: "What needs me first, and why?" });
    await expect(suggestion).toBeVisible();

    const editor = popover.getByTestId("chat-input-editor");
    await suggestion.click();
    await expect(editor).toContainText("What needs me first, and why?");

    await sendMessage(popover, "/e2e:simple-message");
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
    // The intro and suggestion only show while the transcript is empty.
    await expect(
      popover.getByText("Ask the coordinator about anything on this screen."),
    ).not.toBeVisible();
  });

  test("Ask about this on a Needs You card opens the copilot with the derived id, chip, prefix and tag (AC .005.1, .005.2, .005.4, .005.5)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Ask About Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    const title = "Ask About This Card";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      title,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an active session for the clarification task");
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the Needs you screen is opened",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const card = testPage.getByTestId(`needs-you-item-${task.id}`);
    await expect(card).toBeVisible();

    const coordinatorRead = waitForHttp(testPage, "GET", COORDINATOR_READ);
    const conversationOpened = waitForHttp(testPage, "POST", CONVERSATION_OPENED);
    await card.getByRole("button", { name: "Ask about this" }).click();
    await coordinatorRead;
    await conversationOpened;

    const popover = testPage.getByTestId("coordinator-copilot-popover");
    await expect(popover).toBeVisible();
    await expect(popover.getByText(`about ${title}`, { exact: false })).toBeVisible();

    const editor = popover.getByTestId("chat-input-editor");
    await expect(editor).toContainText(`Why is ${title} here?`);

    await sendMessage(popover, `Why is ${title} here?`);
    const aboutTag = popover.getByTestId("coordinator-about-tag").last();
    await expect(aboutTag).toContainText(title);
    await expect(popover.getByText(`Why is ${title} here?`, { exact: true }).last()).toBeVisible();
    // This prompt has no /e2e: scenario prefix, so the mock agent's generic
    // fallback runs (randomized steps, deterministic closing line). The
    // session stays RUNNING until it finishes and rejects a concurrent
    // prompt, so wait for turn 1 to complete before sending a second message.
    await expect(popover.getByText("Everything looks good!", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    await popover.getByRole("button", { name: "Remove" }).click();
    // Scoped to the composer chip's own Remove button rather than a loose
    // `about <title>` text match: the transcript's generic-fallback closing
    // line echoes the sent prompt (including its "About <title>: " wire
    // prefix) verbatim, so a text-based assertion here collides with content
    // that legitimately stays on screen after the chip is gone.
    await expect(popover.getByRole("button", { name: "Remove" })).not.toBeVisible();

    // Removing the chip must drop the wire prefix, not just the composer
    // hint: the next sent message gets no `About <id>: ` tag in the transcript.
    await sendMessage(popover, "/e2e:simple-message");
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
    await expect(popover.getByTestId("coordinator-about-tag")).toHaveCount(1);
  });

  test("closes on Escape returning focus to the launcher, and a reload reopens the same transcript without starting a turn (AC .004.7)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Escape Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    await sendMessage(popover, "/e2e:simple-message");
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    const launcher = testPage.getByTestId("coordinator-copilot-launcher");
    await testPage.keyboard.press("Escape");
    await expect(popover).not.toBeVisible();
    await expect(launcher).toBeFocused();

    // Armed before reload so it observes the fresh socket the reload opens
    // (page.on("websocket") only fires for sockets opened after this call).
    const gatewayRequests = captureGatewayRequests(testPage);
    await testPage.reload();
    await testPage.waitForLoadState("networkidle");

    const reopened = await openCopilot(testPage);
    await expect(
      reopened.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 20_000 });

    // automaticRecovery={false} must keep resume/restore manual-only through
    // reload and reopen (AC .002.2, .002.4); only an explicit Send may launch.
    const launchIntents = gatewayRequests
      .filter((request) => request.action === "session.launch")
      .map((request) => request.payload.intent);
    expect(launchIntents).not.toContain("resume");
    expect(launchIntents).not.toContain("restore");

    await sendMessage(reopened, "/e2e:simple-message");
    await expect(
      reopened.getByText("simple mock response for e2e testing", { exact: false }).nth(1),
    ).toBeVisible({ timeout: 20_000 });
  });

  test("Stop ends a turn that kept running after the popover was closed and reopened (AC .004.5)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Stop After Reopen Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);
    await sendMessage(popover, "/slow 20s");
    await expect(popover.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    const launcher = testPage.getByTestId("coordinator-copilot-launcher");
    await testPage.keyboard.press("Escape");
    await expect(popover).not.toBeVisible();
    await expect(launcher).toHaveAccessibleName(`${coordinator.name} is working`);

    await launcher.click();
    const reopened = testPage.getByTestId("coordinator-copilot-popover");
    await expect(reopened).toBeVisible();

    const cancelButton = reopened.getByTestId("cancel-agent-button");
    await expect(cancelButton).toBeVisible({ timeout: 10_000 });
    await cancelButton.click();

    await expect(cancelButton).not.toBeVisible({ timeout: 15_000 });
    await expect(launcher).toHaveAccessibleName(`Chat with ${coordinator.name}`, {
      timeout: 15_000,
    });
  });

  test("fits within the viewport at 1200px, clear of the sidebar (Acceptance: screens leave room for the popover)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1200, height: 800 });
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Layout Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);

    const box = await popover.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.width).toBeLessThanOrEqual(420);
    expect(box!.height).toBeLessThanOrEqual(550);
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThanOrEqual(1200);
    expect(box!.y + box!.height).toBeLessThanOrEqual(800);

    const sidebarRow = testPage.getByTestId(`sidebar-coordinator-${coordinator.id}`);
    await expect(sidebarRow).toBeVisible();
    const sidebarBox = await sidebarRow.boundingBox();
    expect(sidebarBox).not.toBeNull();
    expect(sidebarBox!.x + sidebarBox!.width).toBeLessThanOrEqual(box!.x);
  });

  test("a failure opening the conversation shows no composer and no session recovery feedback (AC .004.6, reachable row)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Recovery Feedback Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.route(CONVERSATION_OPENED, async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "simulated conversation open failure" }),
      });
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const coordinatorRead = waitForHttp(testPage, "GET", COORDINATOR_READ);
    const launcher = testPage.getByTestId("coordinator-copilot-launcher");
    await expect(launcher).toBeVisible({ timeout: 10_000 });
    await launcher.click();
    await coordinatorRead;

    const popover = testPage.getByTestId("coordinator-copilot-popover");
    await expect(popover).toBeVisible();
    await expect(popover.getByTestId("copilot-open-error")).toBeVisible();
    await expect(popover.getByTestId("chat-input-editor")).toHaveCount(0);
    await expect(popover.getByTestId("session-recovery-error")).toHaveCount(0);
  });

  test("a session that has ended shows session-recovery-error while a Needs you item stays clickable, and Retry opens a new conversation (AC .004.6)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Ended Session Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    // An unrelated task with a blocked clarification question, so the Needs
    // you list has a real, clickable item independent of the coordinator
    // conversation this test ends.
    const needsYouTitle = "Needs You Stays Live";
    const needsYouTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      needsYouTitle,
      seedData.agentProfileId,
      {
        description: "/e2e:clarification",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!needsYouTask.session_id) {
      throw new Error("expected an active session for the clarification task");
    }
    await waitForSessionState(apiClient, {
      taskId: needsYouTask.id,
      sessionId: needsYouTask.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "clarification session should block before the ended-session copilot check",
      timeout: 60_000,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const needsYouCard = testPage.getByTestId(`needs-you-item-${needsYouTask.id}`);
    await expect(needsYouCard).toBeVisible();
    const needsYouAction = needsYouCard.getByRole("button", { name: "Ask about this" });
    await expect(needsYouAction).toBeVisible();

    const { popover, sessionId, taskId } = await openCopilotCapturingSession(testPage);
    await sendMessage(popover, "/e2e:simple-message");
    await expect(
      popover.getByText("simple mock response for e2e testing", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });

    // session.stop needs a live execution to cancel: let the turn settle off
    // RUNNING first, as tests/terminal/terminal-ended-session.spec.ts does.
    const state = async () => {
      const { sessions } = await apiClient.listTaskSessions(taskId);
      return sessions.find((session) => session.id === sessionId)?.state ?? "";
    };
    await expect
      .poll(state, { timeout: 30_000, message: "Waiting for the coordinator turn to settle" })
      .not.toBe("RUNNING");

    await apiClient.stopSession({ session_id: sessionId, force: true });
    await expect
      .poll(state, { timeout: 30_000, message: "Waiting for the coordinator session to end" })
      .toMatch(/FAILED|CANCELLED|COMPLETED/);

    const banner = popover.getByTestId("session-recovery-error");
    await expect(banner).toBeVisible();
    // The Coordinator lists stay usable throughout: the banner does not
    // block interaction elsewhere on the page.
    await expect(needsYouAction).toBeEnabled();

    const retryOpened = waitForHttp(testPage, "POST", CONVERSATION_OPENED);
    await popover.getByTestId("ensure-session-error-retry").click();
    const retryResponse = await retryOpened;
    const retryBody = (await retryResponse.json()) as { task_id: string; session_id: string };
    expect(retryBody.task_id).not.toBe(taskId);

    await expect(banner).toHaveCount(0);
    await expect(
      popover.getByText("Ask the coordinator about anything on this screen."),
    ).toBeVisible();
  });
});

test.describe("Coordinator copilot: permission requests", () => {
  test.beforeAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "false" });
  });
  test.afterAll(async ({ backend }) => {
    await backend.restart({ AGENTCTL_AUTO_APPROVE_PERMISSIONS: "true" });
  });

  test("a tool call requiring permission renders Approve/Deny in the popover and executes on approval (AC .003.9)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(60_000);
    const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
      name: "Permission Coordinator",
      agent_profile_id: seedData.agentProfileId,
      executor_profile_id: seedData.worktreeExecutorProfileId,
    });

    await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
    const popover = await openCopilot(testPage);

    await sendMessage(popover, "/e2e:permission-flow");

    await expect(popover.getByTestId("permission-action-row")).toHaveCount(1, { timeout: 30_000 });
    await popover.getByTestId("permission-approve").click();

    await expect(
      popover.getByText("Permission was granted and command executed.", { exact: false }),
    ).toBeVisible({ timeout: 30_000 });
  });
});
