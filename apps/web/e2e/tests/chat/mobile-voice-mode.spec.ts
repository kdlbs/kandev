import { test, expect } from "../../fixtures/test-base";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

// Mobile (Pixel 5) regression coverage for the voice-mode entry point and
// the chat composer's mic button. Filename matches /mobile-.*\.spec\.ts/ so
// the `mobile-chrome` Playwright project picks it up.
//
// Two assertions:
// 1. The Voice Mode settings page renders all cards at mobile width when
//    navigated to directly — guards against the "setting missing" symptom.
// 2. The mic button mounts on the mobile chat composer with the coarse-pointer
//    override: a user with stored `voiceMode.mode = "hold"` gets toggle
//    behaviour (data-effective-mode = "toggle") and a 40px touch target.

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

  test("Voice Mode settings page renders every card at mobile width", async ({ testPage }) => {
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
    // (The legacy `button[data-sidebar="trigger"]` mobile settings-sidebar
    // trigger was removed by the unified-AppSidebar overhaul — settings nav is
    // now the footer-gear takeover, and the global rail is hidden on mobile.)
  });

  test("voice input mic button mounts on the mobile chat composer with the coarse-pointer touch target and effective toggle mode", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // Seed hold mode so the coarse-pointer override assertion is non-vacuous:
    // data-mode="hold" (stored pref) vs data-effective-mode="toggle" (override)
    // proves the coarse-pointer branch fired, not that the default was toggle.
    await apiClient.saveUserSettings({
      voice_mode: {
        enabled: true,
        engine: "auto",
        language: "auto",
        mode: "hold",
        auto_send: false,
        whisper_web_model: "base",
      },
    });

    const task = await seedTask(apiClient, seedData, "Mobile voice button");
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    const micButton = testPage.getByTestId("voice-input-button");
    await expect(micButton).toBeVisible({ timeout: 15_000 });

    // Pixel 5 advertises `(pointer: coarse)`. Three signals that the coarse-
    // pointer override fired:
    //   - data-mode="hold" (the stored user preference)
    //   - data-effective-mode="toggle" (the override: hold→toggle on coarse)
    //   - h-10 w-10 (40px touch target, only set when isCoarsePointer=true)
    await expect(micButton).toHaveAttribute("data-mode", "hold");
    await expect(micButton).toHaveAttribute("data-effective-mode", "toggle");
    await expect(micButton).toHaveClass(/(^|\s)h-10(\s|$)/);
    await expect(micButton).toHaveClass(/(^|\s)w-10(\s|$)/);
  });
});
