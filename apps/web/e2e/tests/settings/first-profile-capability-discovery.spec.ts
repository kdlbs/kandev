import { test, expect } from "../../fixtures/test-base";
import {
  chooseAdvertisedModel,
  cleanupFirstProfile,
  enableAmpMockProvider,
  expectAmpAvailableWithoutSavedRow,
  expectAmpBaselineAbsent,
  expectFirstDraftRequestEvidence,
  expectPersistedFirstProfile,
  expectReadyDiscovery,
  observeProfileDiscoveryRequests,
  openFirstProfileDraft,
  removeDisposableAmpRows,
  saveSettingsPage,
} from "../../helpers/first-profile-capability-discovery";

/**
 * Issue #5023: a registered agent with no saved parent row must still discover
 * models, resolve dependent options, and save its first profile. The fixture
 * exercises the real backend probe and resolve endpoints; Playwright never
 * fulfills a capability response.
 */
test.describe("First profile capability discovery", () => {
  test("creates the first profile without a persisted parent row", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(180_000);
    const requests = observeProfileDiscoveryRequests(testPage);

    await expectAmpBaselineAbsent(apiClient);
    await enableAmpMockProvider(backend);
    try {
      await removeDisposableAmpRows(apiClient);
      await expectAmpAvailableWithoutSavedRow(apiClient);

      await openFirstProfileDraft(testPage);
      // A new draft starts stale: discovery is user-triggered.
      await testPage.getByTestId("profile-refresh-capabilities").click();
      await expectReadyDiscovery(testPage);

      const chosenModel = await chooseAdvertisedModel(testPage);
      expect(chosenModel).not.toBe("");

      // Discovery must not persist the parent before the explicit Save.
      const { agents: agentsDuringDraft } = await apiClient.listAgents();
      expect(agentsDuringDraft.some((agent) => agent.name === "amp-acp")).toBe(false);

      await testPage.getByTestId("profile-name-input").fill("First Amp profile");
      await saveSettingsPage(testPage);

      const persisted = await expectPersistedFirstProfile(
        apiClient,
        "First Amp profile",
        chosenModel,
      );
      expect(persisted.profileId).not.toBe("");
      expect(persisted.model).not.toBe("");

      await testPage.goto(`/settings/agents/amp-acp/profiles/${persisted.profileId}`);
      await expect(testPage.getByTestId("profile-name-input")).toHaveValue("First Amp profile", {
        timeout: 20_000,
      });
      await expect(
        testPage.getByRole("button", { name: "Profile start model settings" }),
      ).toBeVisible();

      expectFirstDraftRequestEvidence(requests.requests);
    } finally {
      requests.dispose();
      await cleanupFirstProfile(apiClient, backend);
    }
  });
});
