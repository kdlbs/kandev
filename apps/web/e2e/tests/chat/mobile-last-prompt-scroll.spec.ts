// Mobile always shows both scroll buttons (scroll-to-last-prompt and
// scroll-to-start); the anchored bar is a desktop-only affordance and never
// renders on mobile, even when "Show anchored prompt bar" is enabled.
import { test, expect } from "../../fixtures/test-base";
import {
  FIRST_PROMPT_MARKER,
  LAST_PROMPT_MARKER,
  seedScrolledPastLastPrompt,
} from "./last-prompt-scroll-helpers";

test.afterEach(async ({ apiClient }) => {
  await apiClient.saveUserSettings({
    show_anchored_prompt_bar: false,
    show_scroll_to_last_prompt: true,
    show_scroll_to_start: false,
    show_transcript_auto_scroll_control: true,
  });
});

test("mobile falls back to the scroll buttons even with the anchored bar configured", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(90_000);
  await apiClient.saveUserSettings({ show_anchored_prompt_bar: true });
  const session = await seedScrolledPastLastPrompt(
    testPage,
    apiClient,
    seedData,
    "mobile-last-prompt-scroll",
    { sendViaButton: true },
  );
  const chat = session.activeChat();
  const lastMarker = chat.getByText(LAST_PROMPT_MARKER, { exact: false }).first();
  const firstMarker = chat.getByText(FIRST_PROMPT_MARKER, { exact: false }).first();
  await expect(lastMarker).not.toBeInViewport();
  await expect(firstMarker).not.toBeInViewport();

  // No anchored bar on mobile regardless of the configured preference.
  await expect(chat.getByTestId("anchored-last-prompt-bar")).toHaveCount(0);

  const lastPromptButton = chat.getByTestId("scroll-to-last-prompt-button");
  await expect(lastPromptButton).toBeVisible();
  await lastPromptButton.click();
  await expect(lastMarker).toBeInViewport({ timeout: 10_000 });

  const startButton = chat.getByTestId("scroll-to-start-button");
  await expect(startButton).toBeVisible();
  await startButton.click();
  await expect(firstMarker).toBeInViewport({ timeout: 10_000 });
});
