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
    await expect(popover.getByText(`about ${title}`, { exact: false })).not.toBeVisible();

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

  // RESIDUAL, not a missing test: AC-COORDINATOR-COPILOT-004.6's "session
  // exists but cannot start or resume" row is currently unreachable, not
  // merely untested. `ReadyBody` passes `automaticRecovery={false}` to
  // `QuickChatSessionView` (required by AC .002.2/.002.4/.004.5 — no
  // automatic resume/restore on open or reload), which maps to
  // `skipAutomaticRecovery: true` in `useSessionResumption`. The only two
  // writers of the `error`/`notice` state `SessionRecoveryFeedback` renders
  // from are the auto-resume effect itself — which returns immediately
  // whenever `skipAutomaticRecovery` is true
  // (apps/web/hooks/domains/session/use-session-resumption.ts:511-520) — and
  // a manual retry reachable only from inside the banner those two writers
  // would populate: a closed loop with no external trigger. Separately,
  // `QuickChatSessionView`'s non-passthrough render branch never mounts
  // `recoveryFeedback` at all (apps/web/components/quick-chat/quick-chat-session-view.tsx:129-152),
  // a pre-existing gap that predates this stack. See the task plan's Build
  // round 2 RESIDUAL entry for the full trace; the conductor is filing a
  // follow-up for the coordinator-local failed-session display and a WP-2
  // backend defect (the conversation route can return a FAILED session).
  test.skip("a session that cannot start or resume shows SessionRecoveryFeedback while the lists stay live (AC .004.6, residual)", async () => {});
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
