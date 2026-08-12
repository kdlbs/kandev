import { expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { test } from "../../fixtures/test-base";
import { KanbanPage } from "../../pages/kanban-page";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import { seedAvailableCommands } from "../../helpers/session-store";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";

type PersistedAttachment = {
  type?: string;
  name?: string;
  mime_type?: string;
  delivery_mode?: string;
};

/**
 * CLI-mode parity: PassthroughToolbar mounts above the PTY in passthrough
 * sessions and exposes the kandev compose box + Stop button that ACP sessions
 * get via ChatInputArea. These tests verify:
 *
 *  1. The toolbar renders for passthrough sessions.
 *  2. The Chat toggle + send path forwards the typed message to the agent's
 *     stdin (mock-agent echoes it as "Processed: <text>").
 *  3. No dedicated Stop button — users cancel via Ctrl-C in the xterm terminal
 *     (the toolbar intentionally omits a duplicate control).
 */
test.describe("CLI mode: passthrough toolbar", () => {
  /** Create a passthrough agent profile and return its ID. */
  async function createPassthroughProfile(apiClient: ApiClient, name: string): Promise<string> {
    const { agents } = await apiClient.listAgents();
    if (agents.length === 0) throw new Error("no agents registered in this e2e profile");
    const profile = await apiClient.createAgentProfile(agents[0].id, name, {
      model: "mock-fast",
      auto_approve: true,
      cli_passthrough: true,
    });
    return profile.id;
  }

  /**
   * Navigate to the kanban, click the task card, and wait for the passthrough
   * terminal to finish loading.
   */
  async function openTaskAndWaitForTerminal(
    testPage: import("@playwright/test").Page,
    kanban: KanbanPage,
    session: SessionPage,
    taskTitle: string,
  ) {
    const card = kanban.taskCardByTitle(taskTitle);
    await expect(card).toBeVisible({ timeout: 20_000 });
    await card.click();
    await expect(testPage).toHaveURL(/\/t\//, { timeout: 15_000 });
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
  }

  function passthroughEditor(testPage: import("@playwright/test").Page) {
    return testPage.getByTestId("passthrough-composer").locator(".tiptap.ProseMirror");
  }

  function passthroughComposer(testPage: import("@playwright/test").Page) {
    return testPage.getByTestId("passthrough-composer");
  }

  async function waitForPassthroughPromptable(
    apiClient: ApiClient,
    taskId: string,
    sessionId: string,
  ) {
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(taskId);
          return sessions.find((s) => s.id === sessionId)?.state ?? null;
        },
        { timeout: 20_000, message: "Wait for passthrough session to accept a follow-up" },
      )
      .toBe("WAITING_FOR_INPUT");
  }

  async function waitForPersistedAttachment(
    apiClient: ApiClient,
    sessionId: string,
    content: string,
  ): Promise<PersistedAttachment> {
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(sessionId);
          const message = messages.find(
            (m) => m.author_type === "user" && m.content.includes(content),
          );
          const attachments = message?.metadata?.attachments;
          return Array.isArray(attachments) ? attachments.length : 0;
        },
        { timeout: 15_000, message: `Wait for persisted attachment on "${content}"` },
      )
      .toBeGreaterThan(0);

    const { messages } = await apiClient.listSessionMessages(sessionId);
    const message = messages.find((m) => m.author_type === "user" && m.content.includes(content));
    const attachments = message?.metadata?.attachments;
    if (!Array.isArray(attachments)) throw new Error("Persisted user message has no attachments");
    return attachments[0] as PersistedAttachment;
  }

  test("toolbar renders for passthrough sessions on the task page", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Render");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Render Task", profileId, {
      description: "hello toolbar",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Render Task");

    await expect(testPage.getByTestId("passthrough-toolbar")).toBeVisible({ timeout: 10_000 });
    await expect(testPage.getByTestId("passthrough-toggle-composer")).toBeVisible({
      timeout: 5_000,
    });

    const chatToggle = testPage.getByTestId("passthrough-toggle-composer");
    await chatToggle.hover();
    const tooltip = testPage.getByRole("tooltip").filter({ hasText: "Shortcut:" });
    await expect(tooltip).toContainText(/Ctrl\+Shift\+Y|Cmd\+Shift\+Y/);

    await chatToggle.click();
    await expect(testPage.getByTestId("passthrough-composer")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("plan-mode-toggle-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-attachments-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-context-button")).toBeVisible();
    await expect(testPage.getByTestId("toolbar-item-mcp")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-mode")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-model")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-reset-context")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-enhance")).toHaveCount(0);

    await chatToggle.click();
    await expect(testPage.getByTestId("passthrough-composer")).toBeHidden({ timeout: 5_000 });
  });

  test("narrow toolbar wraps complete PR and provider controls without overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 320, height: 800 });
    await apiClient.mockAzureDevOpsSeed({
      authenticated: true,
      user: { ok: true, id: "user-1", displayName: "Ada Reviewer" },
      pullRequests: [
        {
          id: 42,
          title: "Azure companion PR",
          status: "active",
          isDraft: false,
          sourceBranch: "refs/heads/feature/narrow-toolbar",
          targetBranch: "refs/heads/main",
          author: { id: "user-1", displayName: "Ada Reviewer" },
          projectId: "project-1",
          projectName: "Platform",
          repositoryId: "azure-repo-1",
          repositoryName: "api",
          webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
          apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
        },
      ],
      feedback: {
        "42": {
          pullRequest: {
            id: 42,
            title: "Azure companion PR",
            status: "active",
            isDraft: false,
            sourceBranch: "refs/heads/feature/narrow-toolbar",
            targetBranch: "refs/heads/main",
            author: { id: "user-1", displayName: "Ada Reviewer" },
            projectId: "project-1",
            projectName: "Platform",
            repositoryId: "azure-repo-1",
            repositoryName: "api",
            webUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/42",
            apiUrl: "https://dev.azure.com/acme/Platform/_apis/git/pullrequests/42",
          },
          reviewers: [],
          threads: [],
          linkedWorkItems: [],
          policies: [],
          reviewState: "pending",
          policyState: "pending",
        },
      },
    });
    await apiClient.setAzureDevOpsConfig(seedData.workspaceId, {
      organizationUrl: "https://dev.azure.com/acme",
      pat: "azure-test-pat",
    });
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "azure_devops",
      provider_repo_id: "azure-repo-1",
      provider_host: "dev.azure.com/acme",
      provider_owner: "project-1",
      provider_name: "azure-repo-1",
    });
    const profileId = await createPassthroughProfile(apiClient, "CLI Narrow Toolbar");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Narrow Toolbar Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await apiClient.associateAzureDevOpsTaskPR(
      seedData.workspaceId,
      task.id,
      seedData.repositoryId,
      42,
    );
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("test-user");
    const githubPR = {
      workspace_id: seedData.workspaceId,
      task_id: task.id,
      owner: "acme",
      repo: "demo",
      pr_number: 99,
      pr_url: "https://github.com/acme/demo/pull/99",
      pr_title: "Keep passthrough controls contained",
      head_branch: "feature/narrow-toolbar",
      base_branch: "main",
      author_login: "test-user",
      state: "open",
      checks_state: "pending",
      checks_total: 5,
      checks_passing: 0,
      review_state: "pending",
      pending_review_count: 1,
    };
    await apiClient.mockGitHubAssociateTaskPR(githubPR);
    await apiClient.updateTaskCIAutomationOptions(task.id, {
      auto_fix_enabled: true,
      auto_merge_enabled: true,
      prompt_on_review_requested: true,
      prompt_on_merged: true,
      prompt_on_closed: true,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    const row = testPage.getByTestId("passthrough-status-row");
    const chip = row.getByTestId("pr-status-chip");
    await expect(chip.getByTestId("pr-status-auto-fix-chip")).toBeVisible();
    await expect(chip.getByTestId("pr-status-auto-merge-chip")).toBeVisible();
    await expect(chip.getByTestId("pr-status-pr-events-chip")).toHaveText("PR events 3/3");
    await expect(row.getByRole("link", { name: /Azure PR 42/ })).toBeVisible();

    for (const width of [320, 280]) {
      await testPage.setViewportSize({ width, height: 800 });
      await expect(row).toHaveCSS("flex-wrap", "wrap");
      expect(
        await row.evaluate((element) => {
          const bounds = element.getBoundingClientRect();
          const controls = Array.from(element.querySelectorAll("button, a")).filter((control) => {
            const rect = control.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          });
          return (
            element.scrollWidth <= element.clientWidth &&
            controls.every((control) => {
              const rect = control.getBoundingClientRect();
              return rect.left >= bounds.left - 1 && rect.right <= bounds.right + 1;
            })
          );
        }),
      ).toBe(true);
      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);
    }

    expect(
      await chip.evaluate((element) => {
        const rect = element.getBoundingClientRect();
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2,
        );
        return hit === element || element.contains(hit);
      }),
    ).toBe(true);

    await apiClient.mockGitHubAssociateTaskPR({ ...githubPR, state: "merged" });
    await expect(row.getByTestId("pr-merged-banner")).toBeVisible();
    expect(await row.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(true);
  });

  test("composer toggle + send forwards message to the agent", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Send");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Send Task", profileId, {
      description: "initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Send Task");

    // Wait for the initial prompt injection to complete
    await session.expectPassthroughHasText("Processed:", 20_000);

    // Open the composer and send a follow-up message
    await testPage.getByTestId("passthrough-toggle-composer").click();
    await expect(testPage.getByTestId("passthrough-composer")).toBeVisible({ timeout: 5_000 });

    const editor = passthroughEditor(testPage);
    await editor.fill("hello from e2e");
    await testPage.getByTestId("submit-message-button").click();

    // The composer closes on successful send
    await expect(testPage.getByTestId("passthrough-composer")).toBeHidden({ timeout: 10_000 });

    // The mock-agent TUI echoes the prompt as "Processed: <text>"
    await session.expectPassthroughHasText("hello from e2e", 15_000);
  });

  test("composer keeps slash literal and does not show command suggestions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Commands");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toolbar Commands Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Commands Task");
    await session.expectPassthroughHasText("Processed:", 20_000);
    await seedAvailableCommands(testPage, task.session_id, [
      { name: "slow", description: "Run slowly" },
      { name: "error", description: "Trigger an error" },
    ]);

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const editor = passthroughEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await editor.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toHaveCount(0);
    await expect(editor).toHaveText("/s");
  });

  test("supports @ prompt/file lookup chips and sends context to passthrough stdin", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(90_000);

    const repoDir = path.join(backend.tmpDir, "repos", "e2e-repo");
    const git = new GitHelper(repoDir, makeGitEnv(backend.tmpDir));
    const contextFile = `ctx/passthrough-context-${Date.now()}.ts`;
    git.createFile(contextFile, "export const passthroughContextMarker = true;\n");
    git.stageAll();
    git.commit("seed passthrough context lookup");

    const promptName = `pt-prompt-${Date.now()}`;
    const promptContent = "PASSTHROUGH_PROMPT_CONTEXT_MARKER";
    await apiClient.createPrompt(promptName, promptContent);
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Context");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar Context Task", profileId, {
      description: "initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Context Task");
    await session.expectPassthroughHasText("Processed:", 20_000);

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const editor = passthroughEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await editor.click();
    await editor.pressSequentially(`context lookup e2e @${promptName}`);
    await expect(testPage.getByText("Mention tasks, files, prompts")).toBeVisible({
      timeout: 5_000,
    });
    await testPage.getByRole("option", { name: new RegExp(promptName) }).click();
    await expect(editor).toContainText(promptName);

    const contextFileName = path.basename(contextFile);
    await editor.pressSequentially(` @${contextFileName}`);
    await expect(testPage.getByText("Mention tasks, files, prompts")).toBeVisible({
      timeout: 10_000,
    });
    await testPage.getByRole("option", { name: new RegExp(contextFileName) }).click();
    await expect(editor).toContainText(contextFileName);

    await testPage.getByTestId("submit-message-button").click();
    await expect(passthroughComposer(testPage)).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText("context lookup e2e", 15_000);
    await session.expectPassthroughHasText("CONTEXT PROMPTS", 15_000);
    await session.expectPassthroughHasText(promptContent, 15_000);
    await session.expectPassthroughHasText("CONTEXT PATHS", 15_000);
    await session.expectPassthroughHasText(contextFile, 15_000);
  });

  test("expands @ Plan context into passthrough stdin instead of sending a literal mention", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Plan Context");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toolbar Plan Context Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    const planContent = "PASSTHROUGH_PLAN_CONTEXT_MARKER";
    await apiClient.wsRequest("task.plan.create", {
      task_id: task.id,
      title: "Plan",
      content: `## Plan\n\n${planContent}`,
      created_by: "user",
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Plan Context Task");
    await session.expectPassthroughHasText("Processed:", 20_000);

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const editor = passthroughEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await editor.click();
    await editor.pressSequentially("@Plan");
    await expect(testPage.getByText("Mention tasks, files, prompts")).toBeVisible({
      timeout: 5_000,
    });
    await testPage.getByRole("option", { name: /^Plan Include the plan as context$/ }).click();
    await expect(editor).toContainText("Plan");

    await testPage.getByTestId("submit-message-button").click();
    await expect(passthroughComposer(testPage)).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText("CONTEXT PLAN", 15_000);
    await session.expectPassthroughHasText(planContent, 15_000);
  });

  test("uploads attachments and delivers them as workspace file paths", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Attachment");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toolbar Attachment Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Attachment Task");
    await session.expectPassthroughHasText("Processed:", 20_000);
    await waitForPassthroughPromptable(apiClient, task.id, task.session_id);

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentPath = path.join(testInfo.outputDir, "passthrough-upload.txt");
    fs.writeFileSync(attachmentPath, "passthrough attachment body");

    await testPage.getByTestId("passthrough-toggle-composer").click();
    const composer = passthroughComposer(testPage);
    await composer.locator('input[type="file"]').setInputFiles(attachmentPath);
    await expect(composer.getByText("passthrough-upload.txt", { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    await passthroughEditor(testPage).fill("read the attached file");
    await testPage.getByTestId("submit-message-button").click();
    await expect(composer).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText("read the attached file", 15_000);
    await session.expectPassthroughHasText("passthrough-upload.txt", 15_000);
    const attachment = await waitForPersistedAttachment(
      apiClient,
      task.session_id,
      "read the attached file",
    );
    expect(attachment).toMatchObject({
      type: "resource",
      name: "passthrough-upload.txt",
      mime_type: "text/plain",
      delivery_mode: "path",
    });
  });

  test("slash stays in the terminal while the dedicated shortcut focuses the composer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar Focus Shortcut");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toolbar Focus Shortcut Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar Focus Shortcut Task");
    await session.expectPassthroughHasText("Processed:", 20_000);
    await waitForPassthroughPromptable(apiClient, task.id, task.session_id);

    await session.passthroughTerminal.locator(".xterm").click();
    await testPage.keyboard.type("/terminal-slash-e2e");
    await expect(passthroughComposer(testPage)).toHaveCount(0);
    await session.expectPassthroughHasText("/terminal-slash-e2e", 5_000);

    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+y`);

    const composer = passthroughComposer(testPage);
    await expect(composer).toBeVisible({ timeout: 5_000 });
    await expect(passthroughEditor(testPage)).toBeFocused();
  });

  test("toolbar omits a Stop button; cancel is via Ctrl-C in the terminal", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const profileId = await createPassthroughProfile(apiClient, "CLI Toolbar No Stop");

    await apiClient.createTaskWithAgent(seedData.workspaceId, "Toolbar No Stop Task", profileId, {
      description: "initial prompt",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });

    const kanban = new KanbanPage(testPage);
    await kanban.goto();

    const session = new SessionPage(testPage);
    await openTaskAndWaitForTerminal(testPage, kanban, session, "Toolbar No Stop Task");

    await expect(testPage.getByTestId("passthrough-toolbar")).toBeVisible({ timeout: 10_000 });
    // PassthroughToolbar deliberately has no Stop affordance — Ctrl-C in xterm is the path.
    await expect(testPage.getByTestId("passthrough-stop")).toHaveCount(0);

    await session.passthroughTerminal.locator(".xterm").click();
    await testPage.keyboard.press("Control+c");
    // Terminal stays mounted; we only assert the UI never offered a misleading Stop button.
    await expect(testPage.getByTestId("passthrough-terminal")).toBeVisible({ timeout: 5_000 });
  });
});
