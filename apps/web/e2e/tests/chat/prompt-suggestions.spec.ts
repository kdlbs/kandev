import { expect, type Page } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { seedIdleSession } from "../../helpers/session";
import { dwell, injectLatency } from "../../helpers/causal-waits";
import { openQuickChatWithAgent } from "./quick-chat-helpers";
import { NATIVE_SUGGESTION, setPromptSuggestions } from "./prompt-suggestion-helpers";

const FALLBACK_SUGGESTION = "Commit it now";
const EXECUTE_URL = "**/api/v1/utility/execute";

function countExecuteRequests(page: Page) {
  const calls: unknown[] = [];
  page.on("request", (request) => {
    if (request.url().includes("/api/v1/utility/execute")) calls.push(request.postDataJSON());
  });
  return calls;
}

// @covers AC-UI-PROMPT-SUGGEST-002.1 AC-UI-PROMPT-SUGGEST-002.2 AC-UI-PROMPT-SUGGEST-002.5 AC-UI-PROMPT-SUGGEST-004.1 AC-UI-PROMPT-SUGGEST-004.2 AC-UI-PROMPT-SUGGEST-004.3 AC-UI-PROMPT-SUGGEST-004.5 AC-UI-PROMPT-SUGGEST-004.6 AC-UI-PROMPT-SUGGEST-004.9
test("native suggestion shows as ghost text, fills with Tab, sends with the submit key, and dismisses with Escape", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true, false);
  const executeCalls = countExecuteRequests(testPage);
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
    expect(executeCalls).toHaveLength(0);
  } finally {
    await setPromptSuggestions(apiClient, false, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-004.3
test("the send button sends the visible suggestion from an empty draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true, false);
  try {
    const session = await seedIdleSession(testPage, apiClient, seedData, "Send button suggestion");
    const chat = session.activeChat();
    const ghost = chat.locator("p.has-prompt-suggestion");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", NATIVE_SUGGESTION);

    await chat.getByTestId("submit-message-button").click();
    await expect(chat.getByText(NATIVE_SUGGESTION, { exact: true }).first()).toBeVisible();
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
  } finally {
    await setPromptSuggestions(apiClient, false, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-003.1 AC-UI-PROMPT-SUGGEST-003.2
test("sessions without native suggestions use the utility fallback once per turn", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  // Launched with suggestions off, so the agent was never asked for native
  // suggestions and the session is not native.
  await setPromptSuggestions(apiClient, false, false);
  const bodies: Record<string, unknown>[] = [];
  await testPage.route(EXECUTE_URL, async (route) => {
    bodies.push(route.request().postDataJSON() as Record<string, unknown>);
    await route.fulfill({
      json: { success: true, call_id: "call-stub", response: FALLBACK_SUGGESTION },
    });
  });
  try {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Fallback prompt suggestion",
    );
    const requested = testPage.waitForRequest(EXECUTE_URL);
    await setPromptSuggestions(apiClient, true, true);
    await requested;
    const ghost = session.activeChat().locator("p.has-prompt-suggestion");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", FALLBACK_SUGGESTION);
    expect(bodies).toHaveLength(1);
    // @covers AC-UI-PROMPT-SUGGEST-003.9
    expect(bodies[0]).toMatchObject({
      utility_agent_id: "builtin-suggest-next-prompt",
      session_id: "",
      fallback_agent_profile_id: seedData.agentProfileId,
    });
    expect(String(bodies[0].conversation_history)).toContain("Agent: ");

    await testPage.reload();
    await session.composerReady();
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", FALLBACK_SUGGESTION);
    expect(bodies).toHaveLength(1);
  } finally {
    await setPromptSuggestions(apiClient, false, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-004.5
test("a suggestion that arrives while the user is typing never overwrites the draft", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, false, false);
  await testPage.route(EXECUTE_URL, async (route) => {
    await injectLatency(2_000, "the user starts typing before the fallback suggestion arrives");
    await route.fulfill({
      json: { success: true, call_id: "call-stub", response: FALLBACK_SUGGESTION },
    });
  });
  try {
    const session = await seedIdleSession(
      testPage,
      apiClient,
      seedData,
      "Typing before suggestion",
    );
    const chat = session.activeChat();
    const editor = chat.locator(".tiptap.ProseMirror:visible");
    const ghost = chat.locator("p.has-prompt-suggestion");
    await editor.click();

    const requested = testPage.waitForRequest(EXECUTE_URL);
    const answered = testPage.waitForResponse(EXECUTE_URL);
    await setPromptSuggestions(apiClient, true, true);
    await requested;
    await testPage.keyboard.type("my own draft");
    await answered;
    await expect(editor).toHaveText("my own draft");
    await expect(ghost).toHaveCount(0);

    await testPage.keyboard.press("ControlOrMeta+A");
    await testPage.keyboard.press("Backspace");
    await expect(ghost).toHaveAttribute("data-prompt-suggestion", FALLBACK_SUGGESTION);
  } finally {
    await setPromptSuggestions(apiClient, false, false);
  }
});

// @covers AC-UI-PROMPT-SUGGEST-001.2
test("no suggestion and no request while the preference is off", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, false, true);
  const executeCalls = countExecuteRequests(testPage);
  const session = await seedIdleSession(testPage, apiClient, seedData, "Suggestions off");
  await session.composerReady();
  await dwell(
    testPage,
    1_500,
    "negative-assertion",
    "a native suggestion arrives about 150 ms after the turn; none may appear while the preference is off",
  );
  await expect(session.activeChat().locator("p.has-prompt-suggestion")).toHaveCount(0);
  expect(executeCalls).toHaveLength(0);
  await setPromptSuggestions(apiClient, false, false);
});

// @covers AC-UI-PROMPT-SUGGEST-004.6 AC-UI-PROMPT-SUGGEST-004.8
test("Quick Chat: Escape dismisses the suggestion and keeps the dialog open", async ({
  testPage,
  apiClient,
}) => {
  test.setTimeout(120_000);
  await setPromptSuggestions(apiClient, true, false);
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
    await setPromptSuggestions(apiClient, false, false);
  }
});
