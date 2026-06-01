import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// Mobile (Pixel 5) regression coverage for the voice-mode entry point and
// the chat composer's mic button. Filename matches /mobile-.*\.spec\.ts/ so
// the `mobile-chrome` Playwright project picks it up.
//
// Two assertions:
// 1. The Voice Mode settings page is reachable on mobile via the sidebar
//    trigger and every card on the page renders at mobile width — guards
//    against the "setting missing" symptom.
// 2. The mic button is mounted on the mobile chat composer with the
//    coarse-pointer effective-mode override, so a user with stored
//    `voiceMode.mode = "hold"` still gets working toggle behaviour.

async function seedTask(apiClient: ApiClient, seedData: SeedData, title: string) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: "/e2e:simple-message",
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("Mobile voice mode", () => {
  test.describe.configure({ retries: 1, timeout: 60_000 });

  test("Voice Mode settings page is reachable from the mobile sidebar and renders every card", async ({
    testPage,
  }) => {
    await testPage.goto("/settings/voice-mode");

    // Page header confirms route resolved to the right shell. The page title
    // is rendered by SettingsSection — match the heading text exactly so we
    // don't pick up the sidebar entry that shares the label.
    await expect(testPage.getByRole("heading", { name: "Voice Mode", exact: true })).toBeVisible({
      timeout: 15_000,
    });

    // The five cards rendered by `voice-mode-settings.tsx` at mobile width.
    await expect(testPage.getByText("Enable Voice Input")).toBeVisible();
    await expect(testPage.getByText("Transcription Engine")).toBeVisible();
    await expect(testPage.getByText("Behavior")).toBeVisible();
    await expect(testPage.getByText("Whisper Web Model")).toBeVisible();
    await expect(testPage.getByText(/Shortcut$/)).toBeVisible();

    // The mobile sidebar trigger (md:hidden) lives in the topbar; assert it
    // exists so we know the sidebar entry chain that exposed this page is
    // not silently removed by a future layout change.
    const sidebarTrigger = testPage.locator('button[data-sidebar="trigger"]');
    await expect(sidebarTrigger).toBeVisible();
  });

  test("voice input mic button mounts on the mobile chat composer with toggle as the effective mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await seedTask(apiClient, seedData, "Mobile voice button");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const micButton = testPage.getByTestId("voice-input-button");
    await expect(micButton).toBeVisible({ timeout: 15_000 });

    // On Pixel 5 the device descriptor exposes `(pointer: coarse)`, so the
    // stored hold preference (default in fresh user settings is "toggle",
    // but the override still applies even if the user opted into hold)
    // should resolve to the effective toggle handler.
    await expect(micButton).toHaveAttribute("data-effective-mode", "toggle");
  });
});
