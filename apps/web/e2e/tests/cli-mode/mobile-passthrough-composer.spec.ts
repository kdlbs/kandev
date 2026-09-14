// Filename starts with "mobile-" so this runs in the mobile-chrome project.
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import type { Locator, Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { seedAvailableCommands } from "../../helpers/session-store";
import { SessionPage } from "../../pages/session-page";

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

async function openMobilePassthroughTask(
  testPage: import("@playwright/test").Page,
  apiClient: ApiClient,
  seedData: {
    workspaceId: string;
    workflowId: string;
    startStepId: string;
    repositoryId: string;
  },
  profileName: string,
  taskTitle: string,
) {
  const profileId = await createPassthroughProfile(apiClient, profileName);
  const task = await apiClient.createTaskWithAgent(seedData.workspaceId, taskTitle, profileId, {
    description: "initial prompt",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
  if (!task.session_id) throw new Error("expected passthrough task to start a session");

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForPassthroughLoad(20_000);
  await session.waitForPassthroughLoaded(20_000);
  await session.expectPassthroughHasText("Processed:", 20_000);
  return { task, session };
}

async function expectTouchTarget(control: Locator): Promise<void> {
  const box = await control.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  expect(box.width).toBeGreaterThanOrEqual(44);
  expect(box.height).toBeGreaterThanOrEqual(44);
}

function passthroughComposer(testPage: Page) {
  return testPage.getByTestId("passthrough-composer");
}

function passthroughEditor(testPage: Page) {
  return passthroughComposer(testPage).locator(".tiptap.ProseMirror");
}

async function expectWithinVisualViewport(testPage: Page, control: Locator): Promise<void> {
  const box = await control.boundingBox();
  expect(box).not.toBeNull();
  if (!box) return;
  const viewport = await testPage.evaluate(() => ({
    width: window.visualViewport?.width ?? window.innerWidth,
    height: window.visualViewport?.height ?? window.innerHeight,
  }));
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
}

async function expectSingleUserMessage(
  apiClient: ApiClient,
  sessionId: string,
  marker: string,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.filter((message) => {
          return message.author_type === "user" && message.content.includes(marker);
        }).length;
      },
      { timeout: 15_000, message: `Wait for one persisted user message containing "${marker}"` },
    )
    .toBe(1);
}

async function expectReadyDraftAttachment(
  testPage: Page,
  sessionId: string,
  fileName: string,
): Promise<void> {
  await expect
    .poll(
      async () =>
        testPage.evaluate(
          ({ sessionId: currentSessionId, fileName: currentFileName }) => {
            const raw = sessionStorage.getItem(`kandev.chatDraft.attachments.${currentSessionId}`);
            if (!raw) return false;
            try {
              const attachments = JSON.parse(raw) as Array<{
                attachmentId?: string;
                fileName?: string;
              }>;
              return attachments.some(
                (attachment) =>
                  attachment.fileName === currentFileName && Boolean(attachment.attachmentId),
              );
            } catch {
              return false;
            }
          },
          { sessionId, fileName },
        ),
      { timeout: 15_000, message: `Wait for ready draft attachment "${fileName}"` },
    )
    .toBe(true);
}

test.describe("mobile CLI mode: passthrough composer", () => {
  test("keeps passthrough controls touch-sized without document overflow", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const { session } = await openMobilePassthroughTask(
      testPage,
      apiClient,
      seedData,
      "Mobile CLI Geometry",
      "Mobile CLI Geometry Task",
    );
    const toolbar = testPage.getByTestId("passthrough-toolbar");
    const statusRow = toolbar.getByTestId("passthrough-status-row");

    for (const testId of ["passthrough-toggle-composer", "passthrough-toggle-comments"]) {
      await expectTouchTarget(statusRow.getByTestId(testId));
    }
    const proceed = statusRow.getByTestId("passthrough-proceed-next-step");
    if (await proceed.isVisible()) await expectTouchTarget(proceed);

    await statusRow.getByTestId("passthrough-toggle-composer").tap();
    const composer = toolbar.getByTestId("passthrough-composer");
    await expect(composer).toBeVisible();
    for (const testId of [
      "plan-mode-toggle-button",
      "chat-attachments-button",
      "chat-context-button",
      "submit-message-button",
    ]) {
      await expectTouchTarget(composer.getByTestId(testId));
    }

    expect(
      await testPage.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
    expect(await statusRow.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
    expect(await session.passthroughTerminal.isVisible()).toBe(true);
  });

  test("slash remains literal and only passthrough composer controls are available", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const profileId = await createPassthroughProfile(apiClient, "Mobile CLI Commands");
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile CLI Commands Task",
      profileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected passthrough task to start a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForPassthroughLoad(20_000);
    await session.waitForPassthroughLoaded(20_000);
    await session.expectPassthroughHasText("Processed:", 20_000);
    await seedAvailableCommands(testPage, task.session_id, [
      { name: "slow", description: "Run slowly" },
      { name: "error", description: "Trigger an error" },
    ]);

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const composer = testPage.getByTestId("passthrough-composer");
    const editor = composer.locator(".tiptap.ProseMirror");
    await expect(editor).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("plan-mode-toggle-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-context-button")).toBeVisible();
    await expect(testPage.getByTestId("chat-attachments-button")).toBeVisible();
    await expect(testPage.getByTestId("toolbar-item-mcp")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-mode")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-model")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-reset-context")).toHaveCount(0);
    await expect(testPage.getByTestId("toolbar-item-enhance")).toHaveCount(0);

    await editor.fill("/s");

    await expect(testPage.getByRole("listbox", { name: "Command suggestions" })).toHaveCount(0);
    await expect(editor).toHaveText("/s");
  });

  test("context prompt selection creates a chip on mobile", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const promptName = `mobile-pt-prompt-${Date.now()}`;
    const promptContent = "MOBILE_PASSTHROUGH_PROMPT_MARKER";
    await apiClient.createPrompt(promptName, promptContent);
    const { task, session } = await openMobilePassthroughTask(
      testPage,
      apiClient,
      seedData,
      "Mobile CLI Context",
      "Mobile CLI Context Task",
    );

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const composer = passthroughComposer(testPage);
    const editor = passthroughEditor(testPage);
    await expect(editor).toBeVisible({ timeout: 5_000 });

    await editor.click();
    await editor.pressSequentially(`mobile context e2e @${promptName}`);
    const mentionHeading = testPage.getByText("Mention tasks, files, prompts");
    await expect(mentionHeading).toBeVisible({ timeout: 5_000 });
    await expectWithinVisualViewport(testPage, mentionHeading);
    await testPage.getByRole("option", { name: new RegExp(promptName) }).tap();

    await expect(composer.getByText(promptName, { exact: true })).toBeVisible({
      timeout: 5_000,
    });

    await testPage.getByTestId("submit-message-button").tap();
    await expect(composer).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText("mobile context e2e", 15_000);
    await session.expectPassthroughHasText("CONTEXT PROMPTS", 15_000);
    await session.expectPassthroughHasText(promptContent, 15_000);
    if (!task.session_id) throw new Error("expected passthrough task session id");
    await expectSingleUserMessage(apiClient, task.session_id, "mobile context e2e");
  });

  test("sends a selected mobile attachment once", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(90_000);
    const { task, session } = await openMobilePassthroughTask(
      testPage,
      apiClient,
      seedData,
      "Mobile CLI Attachment",
      "Mobile CLI Attachment Task",
    );
    if (!task.session_id) throw new Error("expected passthrough task session id");

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentName = "mobile-passthrough-upload.txt";
    const attachmentPath = path.join(testInfo.outputDir, attachmentName);
    fs.writeFileSync(attachmentPath, "mobile passthrough attachment body");

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const composer = passthroughComposer(testPage);
    const fileChooserPromise = testPage.waitForEvent("filechooser");
    await composer.getByTestId("chat-attachments-button").tap();
    const fileChooser = await fileChooserPromise;
    await fileChooser.setFiles(attachmentPath);
    await expect(composer.getByText(attachmentName, { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expectReadyDraftAttachment(testPage, task.session_id, attachmentName);

    const message = "mobile attachment e2e";
    await passthroughEditor(testPage).fill(message);
    await composer.getByTestId("submit-message-button").tap();
    await expect(composer).toBeHidden({ timeout: 10_000 });

    await session.expectPassthroughHasText(message, 15_000);
    await session.expectPassthroughHasText(attachmentName, 15_000);
    await expectSingleUserMessage(apiClient, task.session_id, message);
  });

  test("restores text, context, and a ready attachment per passthrough session", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    test.setTimeout(120_000);
    const promptName = `mobile-pt-draft-prompt-${Date.now()}`;
    await apiClient.createPrompt(promptName, "MOBILE_PASSTHROUGH_DRAFT_PROMPT");
    const first = await openMobilePassthroughTask(
      testPage,
      apiClient,
      seedData,
      "Mobile CLI Draft A",
      "Mobile CLI Draft A Task",
    );
    if (!first.task.session_id) throw new Error("expected first passthrough task session id");

    const secondProfileId = await createPassthroughProfile(apiClient, "Mobile CLI Draft B");
    const secondTask = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile CLI Draft B Task",
      secondProfileId,
      {
        description: "initial prompt",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!secondTask.session_id) throw new Error("expected second passthrough task session id");

    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const firstComposer = passthroughComposer(testPage);
    await firstComposer.getByTestId("chat-context-button").tap();
    const searchInput = testPage.getByPlaceholder("Search files and prompts...");
    await expect(searchInput).toBeVisible({ timeout: 5_000 });
    await searchInput.fill(promptName);
    await testPage.getByText(promptName, { exact: true }).tap();
    await expect(firstComposer.getByText(promptName, { exact: true })).toBeVisible({
      timeout: 5_000,
    });

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentName = "mobile-passthrough-draft.txt";
    const attachmentPath = path.join(testInfo.outputDir, attachmentName);
    fs.writeFileSync(attachmentPath, "mobile passthrough draft attachment body");
    const fileChooserPromise = testPage.waitForEvent("filechooser");
    await firstComposer.getByTestId("chat-attachments-button").tap();
    const fileChooser = await fileChooserPromise;
    await fileChooser.setFiles(attachmentPath);
    await expect(firstComposer.getByText(attachmentName, { exact: true })).toBeVisible({
      timeout: 10_000,
    });
    await expectReadyDraftAttachment(testPage, first.task.session_id, attachmentName);
    await passthroughEditor(testPage).fill("mobile session draft");

    await testPage.goto(`/t/${secondTask.id}`);
    const secondSession = new SessionPage(testPage);
    await secondSession.waitForPassthroughLoad(20_000);
    await secondSession.waitForPassthroughLoaded(20_000);
    await secondSession.expectPassthroughHasText("Processed:", 20_000);
    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const secondComposer = passthroughComposer(testPage);
    await expect(secondComposer).toBeVisible();
    await expect(secondComposer.locator(".tiptap.ProseMirror")).toHaveText("");
    await expect(secondComposer.getByText(promptName, { exact: true })).toHaveCount(0);
    await expect(secondComposer.getByText(attachmentName, { exact: true })).toHaveCount(0);

    await testPage.goto(`/t/${first.task.id}`);
    const restoredSession = new SessionPage(testPage);
    await restoredSession.waitForPassthroughLoad(20_000);
    await restoredSession.waitForPassthroughLoaded(20_000);
    await restoredSession.expectPassthroughHasText("Processed:", 20_000);
    await testPage.getByTestId("passthrough-toggle-composer").tap();
    const restoredComposer = passthroughComposer(testPage);
    await expect(passthroughEditor(testPage)).toHaveText("mobile session draft");
    await expect(restoredComposer.getByText(promptName, { exact: true })).toBeVisible({
      timeout: 5_000,
    });
    await expect(restoredComposer.getByText(attachmentName, { exact: true })).toBeVisible({
      timeout: 5_000,
    });
  });
});
