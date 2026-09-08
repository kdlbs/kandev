import { test } from "../../fixtures/test-base";
import { openQuickChatSetup } from "./quick-chat-helpers";
import {
  runSavedPromptDeliveryScenario,
  SAVED_PROMPT_NAME,
} from "./quick-chat-saved-prompt-delivery-helpers";

test.describe("Quick Chat saved prompt delivery", () => {
  test.afterEach(async ({ apiClient }) => {
    const { prompts } = await apiClient.listPrompts();
    await Promise.all(
      prompts
        .filter((prompt) => !prompt.builtin && prompt.name === SAVED_PROMPT_NAME)
        .map((prompt) => apiClient.deletePrompt(prompt.id).catch(() => undefined)),
    );
  });

  test("delivers the selected saved prompt on the eager first request", async ({
    testPage,
    apiClient,
  }) => {
    await runSavedPromptDeliveryScenario({
      page: testPage,
      apiClient,
      openSetup: openQuickChatSetup,
      activateEditor: (editor) => editor.click(),
      selectPrompt: (option) => option.click(),
      submitMessage: (button) => button.click(),
    });
  });
});
