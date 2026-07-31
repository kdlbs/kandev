// Regression: with multiple agent tabs on a task, some tabs do not display /
// use the last-prompt navigation affordances (the scroll-to-last-prompt button
// and the anchored prompt bar). Sessions whose panels are created while another
// tab is active mount hidden (dockview toggles the inactive overlay to
// `display:none`), which can leave their transcript edge state out of sync when
// the tab is activated.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { FIRST_PROMPT_MARKER, LAST_PROMPT_MARKER } from "./last-prompt-scroll-helpers";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];

// The dockview session tabs keep every session's chat mounted (hidden), so
// SessionPage.waitForChatIdle's global idle-input locator resolves to multiple
// elements here. Wait for the active panel's own idle input instead.
async function waitForActiveChatIdle(session: SessionPage, timeout = 30_000): Promise<void> {
  const idle = session.activeChat().locator('[data-placeholder^="Continue working"]');
  await expect(idle).toBeVisible({ timeout });
}

async function seedSessionScrolledPastLastPrompt(
  session: SessionPage,
  apiClient: import("../../helpers/api-client").ApiClient,
  sessionId: string,
): Promise<void> {
  const send = async (text: string) => {
    await session.sendMessage(text);
    await waitForActiveChatIdle(session);
  };
  await send(FIRST_PROMPT_MARKER);
  await apiClient.seedAgentMessages(sessionId, 30, "middle filler message");
  await send(LAST_PROMPT_MARKER);
  await apiClient.seedAgentMessages(sessionId, 50);
  await expect(session.activeChat().getByText("filler message 50", { exact: false })).toBeVisible({
    timeout: 15_000,
  });
}

/** Read the active transcript's scroll position. */
async function activeScrollTop(chat: ReturnType<SessionPage["activeChat"]>): Promise<number> {
  return chat.evaluate((root) => {
    const el = root.querySelector<HTMLElement>(".chat-message-list");
    return el ? el.scrollTop : -1;
  });
}

/** Manually scroll the active transcript so a given trailing filler row is in
 * view — the last prompt then sits above the viewport, with content still
 * below (a mid-transcript position, not the bottom). */
async function scrollToFiller(
  chat: ReturnType<SessionPage["activeChat"]>,
  n: number,
): Promise<void> {
  await chat.getByText(`filler message ${n}`, { exact: false }).scrollIntoViewIfNeeded();
  await expect(chat.getByTestId("anchored-last-prompt-bar")).toHaveAttribute("data-state", "open", {
    timeout: 10_000,
  });
}

test.describe("@chat last prompt affordances with multiple agent tabs", () => {
  test.afterEach(async ({ apiClient }) => {
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: false,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: false,
      show_transcript_auto_scroll_control: true,
    });
  });

  test("second agent tab keeps the pinned bar and scroll button across tab switches", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: true,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });

    // 1. Create the task and wait for the first agent session to finish.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi-Agent Last Prompt",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 2. Open the task in the UI and seed the first tab past its last prompt.
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const { sessions: firstSessions } = await apiClient.listTaskSessions(task.id);
    const session1Id = firstSessions[0].id;
    await seedSessionScrolledPastLastPrompt(session, apiClient, session1Id);
    const chat1 = session.activeChat();
    await expect(chat1.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 15_000 },
    );
    await expect(
      chat1.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();

    // 3. Launch a second agent session on the same task.
    const launched = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      },
      60_000,
    );
    await expect(session.sessionTabBySessionId(launched.session_id)).toBeVisible({
      timeout: 20_000,
    });

    // 4. Switch to the second tab and seed it past its last prompt too.
    await session.sessionTabBySessionId(launched.session_id).click();
    await waitForActiveChatIdle(session);
    await seedSessionScrolledPastLastPrompt(session, apiClient, launched.session_id);
    const chat2 = session.activeChat();
    await expect(chat2.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 15_000 },
    );
    await expect(
      chat2.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();

    // 5. Switch back to the first tab — its controls must survive.
    await session.sessionTabBySessionId(session1Id).click();
    await waitForActiveChatIdle(session);
    await expect(chat1.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 15_000 },
    );
    await expect(
      chat1.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();

    // 6. Switch back to the second tab — its controls must survive as well.
    await session.sessionTabBySessionId(launched.session_id).click();
    await waitForActiveChatIdle(session);
    await expect(chat2.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 15_000 },
    );
    await expect(
      chat2.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();
    // Scope to the transcript row itself: the anchored bar renders a
    // shortened copy of the last prompt text and sits in the viewport by
    // design, so a loose text match could resolve to that copy instead.
    const lastPromptRow = chat2
      .locator("[id^='msg-']")
      .filter({ has: testPage.getByText(LAST_PROMPT_MARKER, { exact: true }) });
    await expect(lastPromptRow).not.toBeInViewport();
  });

  test("an agent tab created while inactive lands on the latest transcript when first activated", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: true,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });

    // 1. Create the task and open it — the first session's tab is active.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi-Agent Inactive Tab",
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
    await waitForActiveChatIdle(session);

    // 2. Launch a second agent session while the first tab stays active. Its
    //    dockview panel is created inactive (hidden), and its transcript is
    //    seeded entirely while the panel stays hidden.
    const launched = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: `/e2e:simple-message ${FIRST_PROMPT_MARKER}`,
      },
      60_000,
    );
    await expect(session.sessionTabBySessionId(launched.session_id)).toBeVisible({
      timeout: 20_000,
    });
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          const target = sessions.find((s) => s.id === launched.session_id);
          return target ? DONE_STATES.includes(target.state) : false;
        },
        { timeout: 60_000, message: "Waiting for second session to finish" },
      )
      .toBe(true);
    await apiClient.seedAgentMessages(launched.session_id, 50);

    // 3. First activation: the transcript must land at the bottom (latest
    //    messages), like any chat tab — not stuck at the top where the whole
    //    transcript appears empty of activity.
    await session.sessionTabBySessionId(launched.session_id).click();
    await waitForActiveChatIdle(session);
    const chat2 = session.activeChat();
    await expect(chat2.getByText("filler message 50", { exact: false })).toBeVisible({
      timeout: 15_000,
    });
    const isAtBottom = await chat2.evaluate((root) => {
      const el = root.querySelector<HTMLElement>(".chat-message-list");
      if (!el) return false;
      return el.scrollHeight - el.scrollTop - el.clientHeight < 5;
    });
    expect(isAtBottom, "first activation should scroll the transcript to the bottom").toBe(true);
  });

  test("manually scrolled transcript keeps its last-prompt bar and scroll position across repeated tab switches", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: true,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });

    // 1. First session: seed past its last prompt, then manually scroll to a
    //    mid-transcript position (last prompt above the viewport, content
    //    still below) and record that position.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi-Agent Manual Scroll",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const { sessions: firstSessions } = await apiClient.listTaskSessions(task.id);
    const session1Id = firstSessions[0].id;
    await seedSessionScrolledPastLastPrompt(session, apiClient, session1Id);
    const chat1 = session.activeChat();
    await scrollToFiller(chat1, 35);
    const savedScrollTop1 = await activeScrollTop(chat1);
    await expect(
      chat1.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();

    // 2. Launch a second agent session and seed it the same way.
    const launched = await apiClient.launchSession(
      {
        task_id: task.id,
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        workflow_step_id: seedData.startStepId,
        prompt: "/e2e:simple-message",
      },
      60_000,
    );
    await expect(session.sessionTabBySessionId(launched.session_id)).toBeVisible({
      timeout: 20_000,
    });
    await session.sessionTabBySessionId(launched.session_id).click();
    await waitForActiveChatIdle(session);
    await seedSessionScrolledPastLastPrompt(session, apiClient, launched.session_id);
    const chat2 = session.activeChat();
    await scrollToFiller(chat2, 35);
    const savedScrollTop2 = await activeScrollTop(chat2);
    await expect(
      chat2.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible();

    // 3. Repeated round-trips: every return must keep the last prompt above
    //    the viewport (bar open + scroll button visible) AND preserve the
    //    exact manual scroll position.
    for (let i = 0; i < 4; i++) {
      await session.sessionTabBySessionId(session1Id).click();
      await waitForActiveChatIdle(session);
      await expect(chat1.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
        "data-state",
        "open",
        { timeout: 10_000 },
      );
      await expect(
        chat1.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
      ).toBeVisible();
      expect(Math.abs((await activeScrollTop(chat1)) - savedScrollTop1)).toBeLessThan(10);

      await session.sessionTabBySessionId(launched.session_id).click();
      await waitForActiveChatIdle(session);
      await expect(chat2.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
        "data-state",
        "open",
        { timeout: 10_000 },
      );
      await expect(
        chat2.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
      ).toBeVisible();
      expect(Math.abs((await activeScrollTop(chat2)) - savedScrollTop2)).toBeLessThan(10);
    }
  });

  test("autonomous session with only its task description as a prompt still gets the last-prompt affordances", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    await apiClient.saveUserSettings({
      show_anchored_prompt_bar: true,
      show_scroll_to_last_prompt: true,
      show_scroll_to_start: true,
    });

    // 1. An agent that ran entirely on its own: the task description is the
    //    session's only user message. Bury it under so much autonomous agent
    //    output that the initial fetch window alone can never reach it.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Multi-Agent Autonomous Session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 45_000, message: "Waiting for session to finish" },
      )
      .toBe(true);
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    const { sessions: firstSessions } = await apiClient.listTaskSessions(task.id);
    await waitForActiveChatIdle(session);
    const chat = session.activeChat();
    await apiClient.seedAgentMessages(firstSessions[0].id, 200);
    await expect(chat.getByText("filler message 200", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 2. Reload so the store re-fetches only the newest window: a task
    //    description that is the session's only user message sits 200 agent
    //    messages behind it, beyond the initial fetch.
    await testPage.reload();
    await session.waitForLoad();
    await waitForActiveChatIdle(session);
    const reloadedChat = session.activeChat();
    await expect(reloadedChat.getByText("filler message 200", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 3. The last prompt (the task description) exists in the transcript and
    //    sits above the viewport, so the affordances must be available even
    //    though it is not part of the initially loaded window.
    await expect(
      reloadedChat.getByTestId("chat-status-bar").getByTestId("scroll-to-last-prompt-button"),
    ).toBeVisible({ timeout: 15_000 });
    await expect(reloadedChat.getByTestId("anchored-last-prompt-bar")).toHaveAttribute(
      "data-state",
      "open",
      { timeout: 15_000 },
    );

    // 4. Clicking jumps to the prompt, draining older pages on demand: the
    //    transcript lands near the top, on the task-description message.
    await reloadedChat
      .getByTestId("chat-status-bar")
      .getByTestId("scroll-to-last-prompt-button")
      .click();
    await expect
      .poll(async () => activeScrollTop(reloadedChat), {
        timeout: 30_000,
        message: "Waiting for scroll-to-last-prompt to land near the top",
      })
      .toBeLessThan(100);
  });
});
