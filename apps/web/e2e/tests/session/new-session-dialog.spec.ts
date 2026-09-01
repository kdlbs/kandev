import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionDone } from "../../helpers/session";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import { createRecentUseProfiles, seedTaskSessionRecency } from "./new-session-recency-helpers";

const DONE_STATES = ["COMPLETED", "WAITING_FOR_INPUT"];
const PROMPT_NAME = "e2e-new-agent-prompt";
const PROMPT_CONTENT = "Inspect the failing flow and add a regression test.";

/**
 * Tests for the New Session Dialog UI flow.
 * Verifies that creating a second session via the + menu works end-to-end:
 * dialog opens, environment info is shown, session is created, and a new tab appears.
 */
test.describe("New session dialog", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    for (const prompt of prompts) {
      if (!prompt.builtin && prompt.name === PROMPT_NAME) {
        await apiClient.deletePrompt(prompt.id);
      }
    }
  });

  test("uses task-session recency for the Add Agent default", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(300_000);
    const { profileA, profileB } = await createRecentUseProfiles(apiClient);

    try {
      const { targetTaskId } = await seedTaskSessionRecency({
        testPage,
        apiClient,
        seedData,
        profileA,
        profileB,
      });
      await testPage.goto(`/t/${targetTaskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 30_000 });
      await session.openNewSessionDialog();

      const selector = session.newSessionDialogPage.agentSelector();
      await expect(selector).toContainText(profileB.name);
      await selector.click();
      const options = session.newSessionDialogPage.agentOptions();
      await expect(options.filter({ hasText: profileB.name })).toHaveCount(1);
      await expect(options.first()).toContainText(profileB.name);
      await testPage.keyboard.press("Escape");

      await session.newSessionPromptInput().fill("/e2e:simple-message");
      await session.newSessionStartButton().click();
      await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

      await expect
        .poll(
          async () => {
            const { sessions } = await apiClient.listTaskSessions(targetTaskId);
            return sessions.find((candidate) => candidate.agent_profile_id === profileB.id)
              ?.agent_profile_id;
          },
          { timeout: 30_000, message: "Waiting for the recent profile session" },
        )
        .toBe(profileB.id);
    } finally {
      await apiClient.deleteAgentProfile(profileA.id, true).catch(() => undefined);
      await apiClient.deleteAgentProfile(profileB.id, true).catch(() => undefined);
    }
  });

  test("completed sessions replace the composer with a New Agent action", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Completed Session New Agent Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");
    await apiClient.seedTaskSession(task.id, {
      state: "COMPLETED",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      completedAt: new Date().toISOString(),
    });
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state;
      })
      .toBe("COMPLETED");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    await expect(session.completedSessionBanner()).toBeVisible({ timeout: 15_000 });
    await expect(session.completedSessionNewAgentButton()).toHaveText("New Agent");
    await expect(session.activeChat().locator(".tiptap.ProseMirror")).not.toBeVisible();
    await expect(session.submitButton()).not.toBeVisible();
    await expect(session.recoveryResumeButton()).not.toBeVisible();
    await expect(session.recoveryFreshButton()).not.toBeVisible();

    await session.completedSessionNewAgentButton().click();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
      })
      .toBe(2);
    await expect(session.sessionTabByText("2")).toBeVisible({ timeout: 15_000 });
  });

  test("opens dialog from + menu and shows environment info", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "New Session Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("New Session Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for first session's response to be visible (confirms session loaded)
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the + menu and click "New Session"
    await session.openNewSessionDialog();

    // 5. Verify dialog is visible with environment info
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await expect(session.newSessionEnvironmentInfo()).toBeVisible();
    await expect(session.newSessionPromptInput()).toBeVisible();
    await expect(session.newSessionStartButton()).toBeVisible();
  });

  test("creates second session and new tab appears", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Second Session Tab Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Second Session Tab Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Wait for chat to load before interacting with the + menu
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the new session dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // 5. Fill in prompt and submit
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();

    // 6. Dialog should close
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 7. Verify two session tabs exist (index badge + agent label)
    await expect(session.sessionTabByText("1")).toBeVisible({ timeout: 15_000 });
    await expect(session.sessionTabByText("2")).toBeVisible({ timeout: 15_000 });

    // 8. Verify the new session is active (chat loads fresh context)
    await session.waitForLoad();

    // 9. Verify the backend has two sessions
    const { sessions: allSessions } = await apiClient.listTaskSessions(task.id);
    expect(allSessions).toHaveLength(2);
  });

  test("starts a second session with a staged attachment", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const prompt = "/e2e:simple-message";
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "New Session Attachment Task",
      seedData.agentProfileId,
      {
        description: prompt,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");
    await waitForSessionDone(apiClient, task.id, task.session_id, "Waiting for first session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.openNewSessionDialog();

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentPath = path.join(testInfo.outputDir, "new-session-note.txt");
    fs.writeFileSync(attachmentPath, "new session attachment body");
    const dialog = session.newSessionDialog();
    await dialog.locator('input[type="file"]').setInputFiles(attachmentPath);
    await expect(dialog.getByText("new-session-note.txt", { exact: true })).toBeVisible();
    await session.newSessionPromptInput().fill(prompt);
    await session.newSessionStartButton().click();
    await expect(dialog).not.toBeVisible({ timeout: 10_000 });

    let secondSessionId = "";
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          const second = sessions.find((candidate) => candidate.id !== task.session_id);
          secondSessionId = second?.id ?? "";
          return DONE_STATES.includes(second?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for attached New Agent session" },
      )
      .toBe(true);

    const { messages } = await apiClient.listSessionMessages(secondSessionId);
    const userMessage = messages.find(
      (message) => message.author_type === "user" && message.content.includes(prompt),
    );
    const attachments = userMessage?.metadata?.attachments;
    expect(Array.isArray(attachments) ? attachments[0] : undefined).toMatchObject({
      name: "new-session-note.txt",
    });

    await testPage.reload();
    await session.waitForLoad();
    await expect(
      session.activeChat().getByText("new-session-note.txt", { exact: true }),
    ).toBeVisible({
      timeout: 15_000,
    });
  });

  test("selects a saved prompt without submitting, then launches explicitly", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await apiClient.createPrompt(PROMPT_NAME, PROMPT_CONTENT);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Saved Prompt New Agent Task",
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
        { timeout: 30_000 },
      )
      .toBe(true);

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    const prompt = session.newSessionPromptInput();
    await prompt.pressSequentially("@e2e-new");
    await expect(testPage.getByText(/Mention tasks, files, prompts/i)).toBeVisible();
    await expect(testPage.getByRole("option", { name: new RegExp(PROMPT_NAME) })).toBeVisible();

    await prompt.press("Enter");
    await expect(prompt).toHaveValue(PROMPT_CONTENT);
    await expect(session.newSessionDialog()).toBeVisible();
    expect((await apiClient.listTaskSessions(task.id)).sessions).toHaveLength(1);

    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });
    await expect
      .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions.length, {
        timeout: 30_000,
      })
      .toBe(2);
  });

  test("second session reuses same task environment", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Env Reuse Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // Capture the environment created by first session
    const envBefore = await apiClient.getTaskEnvironment(task.id);
    expect(envBefore).not.toBeNull();

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Env Reuse Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Create second session via the dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });
    await session.newSessionPromptInput().fill("/e2e:simple-message");
    await session.newSessionStartButton().click();
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 10_000 });

    // 5. Wait for second session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.length;
        },
        { timeout: 30_000, message: "Waiting for second session to appear" },
      )
      .toBe(2);

    // 6. Verify both sessions share the same task environment
    const { sessions: allSessions } = await apiClient.listTaskSessions(task.id);
    const envAfter = await apiClient.getTaskEnvironment(task.id);
    expect(envAfter).not.toBeNull();
    expect(envAfter!.id).toBe(envBefore!.id);

    // Both sessions should reference the same environment
    for (const s of allSessions) {
      expect(s.task_environment_id).toBe(envBefore!.id);
    }
  });

  test("dialog cancel does not create a session", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Cancel Dialog Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Cancel Dialog Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open and cancel the dialog
    await session.openNewSessionDialog();
    await expect(session.newSessionDialog()).toBeVisible({ timeout: 5_000 });

    // Type something to verify it doesn't accidentally submit
    await session.newSessionPromptInput().fill("should not create");

    // Click cancel
    const cancelBtn = session.newSessionDialog().getByRole("button", { name: "Cancel" });
    await cancelBtn.click();

    // 5. Verify dialog closed and no new session was created
    await expect(session.newSessionDialog()).not.toBeVisible({ timeout: 5_000 });

    const { sessions } = await apiClient.listTaskSessions(task.id);
    expect(sessions).toHaveLength(1);
  });

  test("+ dropdown lists sessions with status and primary indicators", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // 1. Create task with first session via API
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Session List Task",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // 2. Wait for first session to complete
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return DONE_STATES.includes(sessions[0]?.state ?? "");
        },
        { timeout: 30_000, message: "Waiting for first session to finish" },
      )
      .toBe(true);

    // 3. Navigate to the task
    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const card = kanban.taskCardByTitle("Session List Task");
    await expect(card).toBeVisible({ timeout: 10_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("simple mock response", { exact: false })).toBeVisible({
      timeout: 15_000,
    });

    // 4. Open the + dropdown
    await session.addPanelButton().click();

    // 5. Verify session item is visible with #1 label
    const sessionItems = session.sessionReopenItems();
    await expect(sessionItems.first()).toBeVisible({ timeout: 5_000 });
    await expect(sessionItems.first()).toContainText("#1");

    // 6. Verify the "Agents" label is visible
    const sessionsLabel = testPage.getByText("Agents", { exact: true });
    await expect(sessionsLabel).toBeVisible();
  });
});
