import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { dwell } from "../../helpers/causal-waits";
import { openQuickChatWithAgent } from "./quick-chat-helpers";
import {
  NATIVE_SUGGESTION,
  observeGatewayRequests,
  setPromptSuggestions,
} from "./prompt-suggestion-helpers";

// The mock agent skips the native suggestion for a prompt containing this directive.
const NO_SUGGESTION_DIRECTIVE = "/no-suggestion";

// @covers AC-UI-PROMPT-SUGGEST-002.1 AC-UI-PROMPT-SUGGEST-002.3 AC-UI-PROMPT-SUGGEST-004.1 AC-UI-PROMPT-SUGGEST-004.2 AC-UI-PROMPT-SUGGEST-004.3 AC-UI-PROMPT-SUGGEST-004.5 AC-UI-PROMPT-SUGGEST-004.6 AC-UI-PROMPT-SUGGEST-004.9
test("native suggestion shows as ghost text, fills with Tab, sends with the submit key, and dismisses with Escape", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true);
  try {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Native prompt suggestion",
    );
    const chat = session.activeChat();
    const ghost = chat.locator("p.has-prompt-suggestion");
    const accept = chat.getByTestId("prompt-suggestion-accept");
    const editor = chat.locator(".tiptap.ProseMirror:visible");

    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);
    await expect(accept).toBeVisible();
    await expect(accept).toHaveAttribute("aria-label", new RegExp(NATIVE_SUGGESTION));

    await testPage.reload();
    await session.composerReady();
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    await editor.click();
    await testPage.keyboard.press("Tab");
    await expect(editor).toHaveText(NATIVE_SUGGESTION);
    await expect(ghost).toHaveCount(0);
    await expect(accept).toHaveCount(0);

    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.press("Backspace");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    await testPage.keyboard.press("ControlOrMeta+Enter");
    await expect(chat.getByText(NATIVE_SUGGESTION, { exact: true }).first()).toBeVisible();
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });

    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);
    await editor.click();
    await testPage.keyboard.press("Escape");
    await expect(ghost).toHaveCount(0);
    await expect(accept).toHaveCount(0);
  } finally {
    await setPromptSuggestions(apiClient, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-004.3
test("the send button sends the visible suggestion from an empty draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true);
  try {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Send button suggestion");
    const chat = session.activeChat();
    const ghost = chat.locator("p.has-prompt-suggestion");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    await chat.getByTestId("submit-message-button").click();
    await expect(chat.getByText(NATIVE_SUGGESTION, { exact: true }).first()).toBeVisible();
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
  } finally {
    await setPromptSuggestions(apiClient, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-003.2 AC-UI-PROMPT-SUGGEST-003.3 AC-UI-PROMPT-SUGGEST-004.7
test("a suggestion from an older turn stays hidden after a turn without one, including after a reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true);
  try {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Stale suggestion");
    const chat = session.activeChat();
    const ghost = chat.locator("p.has-prompt-suggestion");
    const editor = chat.locator(".tiptap.ProseMirror:visible");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    // The mock agent suppresses the native suggestion for this turn, so the
    // stored suggestion still belongs to the previous turn.
    await editor.click();
    await testPage.keyboard.type(`${NO_SUGGESTION_DIRECTIVE} summarize`);
    await chat.getByTestId("submit-message-button").click();
    await expect(chat.getByText(`${NO_SUGGESTION_DIRECTIVE} summarize`).first()).toBeVisible();
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    await dwell(
      testPage,
      1_500,
      "negative-assertion",
      "a native suggestion arrives about 150 ms after the turn; none may appear for a suppressed turn",
    );
    await expect(ghost).toHaveCount(0);
    await expect(chat.getByTestId("prompt-suggestion-accept")).toHaveCount(0);

    await testPage.reload();
    await session.composerReady();
    await expect(ghost).toHaveCount(0);
  } finally {
    await setPromptSuggestions(apiClient, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-001.2
test("no suggestion while the preference is off", async ({ testPage, apiClient, seedData }) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, false);
  const session = await seedIdleSession(testPage, apiClient, seedData, "Suggestions off");
  await session.composerReady();
  await dwell(
    testPage,
    1_500,
    "negative-assertion",
    "a native suggestion arrives about 150 ms after the turn; none may appear while the preference is off",
  );
  await expect(session.activeChat().locator("p.has-prompt-suggestion")).toHaveCount(0);
  await setPromptSuggestions(apiClient, false);
});

// @covers AC-UI-PROMPT-SUGGEST-004.6 AC-UI-PROMPT-SUGGEST-004.8
test("Quick Chat: Escape dismisses the suggestion and keeps the dialog open", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true);
  try {
    const dialog = await openQuickChatWithAgent(testPage);
    const ghost = dialog.locator("p.has-prompt-suggestion");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    await dialog.locator(".tiptap.ProseMirror").click();
    await testPage.keyboard.press("Escape");
    await expect(ghost).toHaveCount(0);
    await expect(dialog).toBeVisible();

    await testPage.keyboard.press("Escape");
    await expect(dialog).not.toBeVisible();
  } finally {
    await setPromptSuggestions(apiClient, false);
  }
});

test.describe("agent without native suggestions", () => {
  test.beforeAll(async ({ backend }) => {
    // Prompt queueing stays advertised so this fixture proves the suggestion
    // gate uses its own explicit denial, not the unrelated queueing namespace.
    await backend.restart({ KANDEV_MOCK_AGENT_PROMPT_SUGGESTIONS: "false" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  // @covers AC-UI-PROMPT-SUGGEST-002.1 AC-UI-PROMPT-SUGGEST-003.1
  test("keeps the normal composer and generates no replacement suggestion", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    // Arm before the first navigation so the collector sees the gateway socket.
    const gatewayRequests = observeGatewayRequests(testPage);
    await setPromptSuggestions(apiClient, true);
    const utilityCalls: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().includes("/api/v1/utility/execute")) utilityCalls.push(request.url());
    });
    try {
      const session = await seedIdleSession(testPage, apiClient, seedData, "Unsupported agent");
      const chat = session.activeChat();
      const editor = chat.locator(".tiptap.ProseMirror:visible");
      await dwell(
        testPage,
        1_500,
        "negative-assertion",
        "a native suggestion arrives about 150 ms after the turn; none may appear without native support",
      );
      await expect(chat.locator("p.has-prompt-suggestion")).toHaveCount(0);
      await expect(chat.getByTestId("prompt-suggestion-accept")).toHaveCount(0);
      // No replacement generation may start automatically after the turn,
      // before the typing observation window below.
      expect(utilityCalls).toHaveLength(0);

      // Typing must not request generation or a replacement suggestion. The
      // fixture's own prompt predates this interval, so only new requests count.
      const requestsBeforeTyping = gatewayRequests.length;
      const utilityCallsBeforeTyping = utilityCalls.length;
      const typed = "plain text with <angle> & entities";
      await editor.click();
      await testPage.keyboard.type(typed);
      await expect(editor).toHaveText(typed);
      await dwell(
        testPage,
        1_500,
        "negative-assertion",
        "typing must not cause a prompt or utility request; no event exists for an absence",
      );
      expect(gatewayRequests.slice(requestsBeforeTyping)).not.toContain("message.add");
      expect(utilityCalls.slice(utilityCallsBeforeTyping)).toHaveLength(0);

      await testPage.keyboard.press("ControlOrMeta+A");
      await testPage.keyboard.press("Backspace");
      await testPage.keyboard.press("Tab");
      await expect(editor).toHaveText("");
      await expect(chat.locator("p.has-prompt-suggestion")).toHaveCount(0);
      // Whole-observation assertion: neither the automatic post-turn path nor
      // typing may call a utility agent for a replacement suggestion.
      expect(utilityCalls).toHaveLength(0);
    } finally {
      await setPromptSuggestions(apiClient, false);
    }
  });
});
