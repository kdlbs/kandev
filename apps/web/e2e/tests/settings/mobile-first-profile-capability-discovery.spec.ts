import { test, expect } from "../../fixtures/test-base";
import {
  chooseAdvertisedModel,
  cleanupFirstProfile,
  enableAmpMockProvider,
  expectAmpAvailableWithoutSavedRow,
  expectFirstDraftRequestEvidence,
  expectPersistedFirstProfile,
  expectReadyDiscovery,
  observeProfileDiscoveryRequests,
  openFirstProfileDraft,
  removeDisposableAmpRows,
  saveSettingsPage,
} from "../../helpers/first-profile-capability-discovery";

/**
 * Phone parity for issue #5023: the same first-profile flow through one column,
 * touch-sized refresh/save targets, and no horizontal document overflow.
 */
test.describe("Mobile first profile capability discovery", () => {
  test("creates the first profile from a phone without a persisted parent", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(180_000);
    const requests = observeProfileDiscoveryRequests(testPage);

    await enableAmpMockProvider(backend);
    try {
      await removeDisposableAmpRows(apiClient);
      await expectAmpAvailableWithoutSavedRow(apiClient);

      await openFirstProfileDraft(testPage);

      const refresh = testPage.getByTestId("profile-refresh-capabilities");
      await refresh.scrollIntoViewIfNeeded();
      expect((await refresh.boundingBox())?.height ?? 0).toBeGreaterThanOrEqual(44);
      // A new draft starts stale: discovery is user-triggered.
      await refresh.tap();
      await expectReadyDiscovery(testPage);

      const chosenLabel = await chooseAdvertisedModel(testPage);
      expect(chosenLabel).not.toBe("");

      const { agents: agentsDuringDraft } = await apiClient.listAgents();
      expect(agentsDuringDraft.some((agent) => agent.name === "amp-acp")).toBe(false);

      await testPage.getByTestId("profile-name-input").fill("Mobile Amp profile");
      await saveSettingsPage(testPage);

      const persisted = await expectPersistedFirstProfile(apiClient, "Mobile Amp profile");
      expect(persisted.model).not.toBe("");
      await testPage.goto(`/settings/agents/amp-acp/profiles/${persisted.profileId}`);
      await expect(testPage.getByTestId("profile-name-input")).toHaveValue("Mobile Amp profile", {
        timeout: 20_000,
      });

      expect(
        await testPage.evaluate(
          () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
        ),
      ).toBe(true);

      expectFirstDraftRequestEvidence(requests.requests);
    } finally {
      requests.dispose();
      await cleanupFirstProfile(apiClient, backend);
    }
  });
});
