import { test, expect } from "../../fixtures/test-base";
import { waitForHttp } from "../../helpers/causal-waits";
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

type CreatedProfiles = {
  candidateId?: string;
  dynamicId?: string;
  candidateDeleted?: boolean;
  dynamicDeleted?: boolean;
};

async function createCandidateWithDynamicParent(
  apiClient: ApiClient,
  label: string,
  created: CreatedProfiles,
) {
  const { agents } = await apiClient.listAgents();
  const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
  const candidate = await apiClient.createAgentProfile(agent.id, `${label} candidate`, {
    model: agent.profiles[0].model,
  });
  created.candidateId = candidate.id;
  const dynamic = await apiClient.createDynamicAgentProfile(`${label} selector`, [
    { executionProfileId: candidate.id, ...CANDIDATE_POLICY },
  ]);
  created.dynamicId = dynamic.id;
  return { agent, candidate, dynamic };
}

async function cleanupProfiles(apiClient: ApiClient, created: CreatedProfiles) {
  try {
    if (created.dynamicId && !created.dynamicDeleted) {
      await apiClient.deleteAgentProfile(created.dynamicId, true);
    }
  } finally {
    if (created.candidateId && !created.candidateDeleted) {
      await apiClient.deleteAgentProfile(created.candidateId, true);
    }
  }
}

function profileDeletePath(profileId: string) {
  return new RegExp(`/api/v1/agent-profiles/${profileId}$`);
}

async function confirmProfileDelete(page: Page, agentName: string, profileId: string) {
  await page.goto(`/settings/agents/${agentName}/profiles/${profileId}`);
  await expect(page.getByText("Delete profile", { exact: true })).toBeVisible();
  await page.getByTestId("profile-delete-trigger").click();
  const confirmation = page.getByTestId("agent-profile-delete-confirm-popover");
  await expect(confirmation).toBeVisible();
  const deletion = waitForHttp(page, "DELETE", profileDeletePath(profileId));
  await confirmation.getByTestId("agent-profile-delete-confirm").click();
  return deletion;
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
    const created: CreatedProfiles = {};
    try {
      const { agent, candidate, dynamic } = await createCandidateWithDynamicParent(
        apiClient,
        "Retired",
        created,
      );
      await apiClient.deleteAgentProfile(dynamic.id, true);
      created.dynamicDeleted = true;

      const deleteResponse = await confirmProfileDelete(testPage, agent.name, candidate.id);
      expect(deleteResponse.ok()).toBe(true);
      created.candidateDeleted = true;

      await expect(testPage).toHaveURL(/\/settings\/agents$/);
      await expect(testPage.getByRole("alertdialog")).toHaveCount(0);
      await expect
        .poll(async () => {
          const { agents } = await apiClient.listAgents();
          return agents.some((item) =>
            item.profiles?.some((profile) => profile.id === candidate.id),
          );
        })
        .toBe(false);
    } finally {
      try {
        await cleanupProfiles(apiClient, created);
      } finally {
        await releaseDynamicRouting();
      }
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
    const created: CreatedProfiles = {};
    try {
      const { agent, candidate, dynamic } = await createCandidateWithDynamicParent(
        apiClient,
        "Balanced",
        created,
      );

      const deleteResponse = await confirmProfileDelete(testPage, agent.name, candidate.id);
      expect(deleteResponse.status()).toBe(409);

      const conflictDialog = testPage.getByRole("alertdialog");
      await expect(conflictDialog).toBeVisible();
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

      const forceDelete = waitForHttp(testPage, "DELETE", profileDeletePath(candidate.id), {
        predicate: (response) => response.status() !== 409,
      });
      await conflictDialog.getByRole("button", { name: "Delete Anyway" }).click();
      const forceResponse = await forceDelete;
      expect(forceResponse.ok()).toBe(true);
      created.candidateDeleted = true;
      await expect(testPage).toHaveURL(/\/settings\/agents$/);
    } finally {
      try {
        await cleanupProfiles(apiClient, created);
      } finally {
        await releaseDynamicRouting();
      }
    }
  });
});
