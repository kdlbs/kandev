import type { Locator, Page } from "@playwright/test";
import { expect } from "@playwright/test";
import { SessionPage } from "../../pages/session-page";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export const AGENT_REPLY = "The settled answer contains a useful detail.";

export async function expectAgentMessageHighlight(body: Locator, expectedCount: number) {
  const highlightName = await body.getAttribute("data-agent-message-highlight-name");
  if (!highlightName) throw new Error("Expected an agent message highlight name");
  await expect
    .poll(() =>
      body.evaluate((_, name) => {
        const registry = (
          CSS as typeof CSS & {
            highlights?: { get: (key: string) => { size: number } | undefined };
          }
        ).highlights;
        return registry?.get(name)?.size ?? 0;
      }, highlightName),
    )
    .toBe(expectedCount);
}

export async function clickAgentMessageHighlight(body: Locator, selectedText: string) {
  const position = await body.evaluate((element, text) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const value = node.nodeValue ?? "";
      const start = value.indexOf(text);
      if (start >= 0) {
        const range = document.createRange();
        range.setStart(node, start);
        range.setEnd(node, start + text.length);
        const rangeRect = range.getBoundingClientRect();
        const bodyRect = element.getBoundingClientRect();
        return {
          x: rangeRect.left - bodyRect.left + rangeRect.width / 2,
          y: rangeRect.top - bodyRect.top + rangeRect.height / 2,
        };
      }
      node = walker.nextNode();
    }
    throw new Error(`Could not locate highlighted text: ${text}`);
  }, selectedText);
  await body.click({ position });
}

export async function openSeededAgentReply(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: 'e2e:message("ready")',
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 45_000 });
  await apiClient.seedSessionMessage(task.session_id, {
    type: "message",
    content: AGENT_REPLY,
  });

  const body = session.activeChat().locator(`[data-agent-message-body][data-message-id]`).filter({
    hasText: AGENT_REPLY,
  });
  await expect(body).toBeVisible({ timeout: 15_000 });
  return { task, session, body };
}

export async function selectAgentReplyText(body: ReturnType<SessionPage["activeChat"]>) {
  await body.evaluate((element, selectedText) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const value = node.nodeValue ?? "";
      const start = value.indexOf(selectedText);
      if (start >= 0) {
        const range = document.createRange();
        range.setStart(node, start);
        range.setEnd(node, start + selectedText.length);
        const selection = window.getSelection();
        selection?.removeAllRanges();
        selection?.addRange(range);
        element.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
        return;
      }
      node = walker.nextNode();
    }
    throw new Error(`Could not find selected text: ${selectedText}`);
  }, "settled answer");
}

export async function openSeededQuickChatReply(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const quickChat = await apiClient.startQuickChat(
    seedData.workspaceId,
    seedData.agentProfileId,
    title,
  );
  await page.goto("/");
  await page.waitForLoadState("networkidle");
  const modifier = process.platform === "darwin" ? "Meta" : "Control";
  await page.keyboard.press(`${modifier}+Shift+q`);
  const dialog = page.getByRole("dialog", { name: "Quick Chat" });
  await expect(dialog).toBeVisible({ timeout: 15_000 });
  await expect(dialog.getByTestId("quick-chat-messages")).toBeVisible();
  await expect(dialog.locator(".tiptap.ProseMirror")).toBeVisible({ timeout: 30_000 });

  await apiClient.seedSessionMessage(quickChat.session_id, {
    type: "message",
    content: AGENT_REPLY,
  });
  const body = dialog
    .getByTestId("quick-chat-messages")
    .locator("[data-agent-message-body]")
    .filter({
      hasText: AGENT_REPLY,
    });
  await expect(body).toBeVisible({ timeout: 15_000 });
  return { dialog, body };
}
