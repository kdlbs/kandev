import { test, expect } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

/**
 * A dynamic profile lists concrete profiles as routing candidates. Only a live
 * dynamic profile is a dependency of its candidates: once the dynamic profile
 * is deleted its routes can never run, so deleting the candidate must not be
 * blocked by it. A live reference still appears in the conflict dialog.
 */

const CANDIDATE_POLICY = {
  enabled: true,
  unclassifiedEnabled: false,
  consecutiveFailureThreshold: 0,
};

async function createCandidateWithDynamicParent(apiClient: ApiClient, label: string) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  const candidate = await apiClient.createAgentProfile(agent.id, `${label} candidate`, {
    model: agent.profiles[0].model,
  });
  const dynamic = await apiClient.createDynamicAgentProfile(`${label} selector`, [
    { executionProfileId: candidate.id, ...CANDIDATE_POLICY },
  ]);
  return { agent, candidate, dynamic };
}

async function confirmProfileDelete(page: Page, agentName: string, profileId: string) {
  await page.goto(`/settings/agents/${agentName}/profiles/${profileId}`);
  await expect(page.getByText("Delete profile", { exact: true })).toBeVisible({
    timeout: 15_000,
  });
  await page.getByTestId("profile-delete-trigger").click();
  const confirmation = page.getByTestId("agent-profile-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  await confirmation.getByTestId("agent-profile-delete-confirm").click();
}

test.describe("Agent profile deletion with dynamic profile references", () => {
  test("a deleted dynamic profile does not block deleting its candidate", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);
    const releaseDynamicRouting = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });
    try {
      const { agent, candidate, dynamic } = await createCandidateWithDynamicParent(
        apiClient,
        "Retired",
      );
      await apiClient.deleteAgentProfile(dynamic.id, true);

      await confirmProfileDelete(testPage, agent.name, candidate.id);

      await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
      await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
      await expect
        .poll(async () => {
          try {
            await apiClient.getAgentProfile(candidate.id);
            return "present";
          } catch {
            return "deleted";
          }
        })
        .toBe("deleted");
    } finally {
      await releaseDynamicRouting();
    }
  });

  test("a live dynamic profile is listed in the conflict dialog", async ({
    testPage,
    apiClient,
    backend,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const releaseDynamicRouting = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });
    try {
      const { agent, candidate, dynamic } = await createCandidateWithDynamicParent(
        apiClient,
        "Balanced",
      );

      await confirmProfileDelete(testPage, agent.name, candidate.id);

      const conflictDialog = testPage.getByRole("alertdialog");
      await expect(conflictDialog).toBeVisible({ timeout: 10_000 });
      const section = conflictDialog.getByTestId("profile-conflict-dynamic-profiles");
      await expect(section.getByText("Dynamic profiles (will keep stale routes):")).toBeVisible();
      await expect(section.getByText(dynamic.name)).toBeVisible();
      if (prCapture.capturing) {
        // Capture the settled dialog, not a frame of its open transition.
        await testPage.evaluate(() =>
          Promise.all(
            document
              .getAnimations()
              .filter((animation) => animation.effect?.getComputedTiming().iterations !== Infinity)
              .map((animation) => animation.finished),
          ),
        );
      }
      await prCapture.screenshot("desktop-agent-profile-delete-dynamic-conflict", {
        caption: "Delete conflict dialog naming a live dynamic profile",
      });

      await conflictDialog.getByRole("button", { name: "Delete Anyway" }).click();
      await expect(testPage).toHaveURL(/\/settings\/agents$/, { timeout: 15_000 });
    } finally {
      await releaseDynamicRouting();
    }
  });
});
