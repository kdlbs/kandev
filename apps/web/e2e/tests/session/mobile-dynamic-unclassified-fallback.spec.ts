import { test, expect } from "../../fixtures/test-base";
import {
  DYNAMIC_FALLBACK_DRAFT,
  DYNAMIC_FALLBACK_SUCCESS,
  createDynamicFallbackProfile,
  expectCurrentCandidate,
  retryCurrentDynamicCandidate,
  startDynamicFallbackSession,
  waitForRouteActionRequired,
} from "../../helpers/dynamic-unclassified-fallback";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";

test.describe("mobile: dynamic unclassified fallback", () => {
  test("keeps manual retry reachable before switching on the third matching failure", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });

    try {
      const profile = await createDynamicFallbackProfile(apiClient, seedData, {
        name: `Unclassified mobile ${Date.now().toString(36)}`,
        enabled: true,
      });
      const attempt = await startDynamicFallbackSession(testPage, apiClient, seedData, {
        title: "Unclassified mobile repeated failure",
        prompt: "/e2e:dynamic-unclassified:same:3",
        profileId: profile.dynamicProfile.id,
      });
      const firstFailure = await waitForRouteActionRequired(
        apiClient,
        attempt.taskId,
        attempt.sessionId,
      );
      const retry = attempt.session.activeChat().getByTestId("dynamic-route-retry");
      await expect(retry).toBeVisible();
      await expectCurrentCandidate(
        apiClient,
        attempt.taskId,
        attempt.sessionId,
        profile.dynamicProfile.id,
        profile.firstCandidate.id,
      );

      const secondFailure = await retryCurrentDynamicCandidate({
        page: testPage,
        apiClient,
        taskId: attempt.taskId,
        sessionId: attempt.sessionId,
        mobile: true,
      });
      expect(secondFailure.execution_profile_id).toBe(profile.firstCandidate.id);
      await expect(attempt.session.activeChat().getByTestId("dynamic-route-retry")).toBeVisible();

      await attempt.session.activeChat().getByTestId("dynamic-route-retry").tap();
      await waitForAgentMessage(apiClient, attempt.sessionId, DYNAMIC_FALLBACK_SUCCESS, 120_000);
      await waitForSessionDone(
        apiClient,
        attempt.taskId,
        attempt.sessionId,
        "mobile successor response settled",
      );
      await expect(
        attempt.session.activeChat().getByText(DYNAMIC_FALLBACK_SUCCESS, { exact: true }),
      ).toBeVisible();

      const draft = attempt.session
        .activeChat()
        .locator('.tiptap.ProseMirror[contenteditable="true"]')
        .first();
      await expect(draft).toBeEditable();
      await draft.fill(DYNAMIC_FALLBACK_DRAFT);
      await expect(draft).toHaveText(DYNAMIC_FALLBACK_DRAFT);
      const switched = await expectCurrentCandidate(
        apiClient,
        attempt.taskId,
        attempt.sessionId,
        profile.dynamicProfile.id,
        profile.secondCandidate.id,
      );
      expect(switched?.route_generation).toBeGreaterThan(firstFailure.route_generation ?? 0);
      await assertNoDocumentHorizontalOverflow(testPage);

      await testPage.reload();
      await attempt.session.waitForLoad();
      await expect(
        attempt.session.activeChat().getByText(DYNAMIC_FALLBACK_SUCCESS, { exact: true }),
      ).toBeVisible();
      const reloadedDraft = attempt.session
        .activeChat()
        .locator('.tiptap.ProseMirror[contenteditable="true"]');
      await expect(reloadedDraft).toBeEditable();
      await expect(reloadedDraft).toHaveText(DYNAMIC_FALLBACK_DRAFT);
      const afterReload = await expectCurrentCandidate(
        apiClient,
        attempt.taskId,
        attempt.sessionId,
        profile.dynamicProfile.id,
        profile.secondCandidate.id,
      );
      expect(afterReload?.route_generation).toBe(switched?.route_generation);
      await assertNoDocumentHorizontalOverflow(testPage);
    } finally {
      await apiClient.updateWorkflowStep(seedData.startStepId, {
        disable_unclassified_fallback: false,
      });
      await releaseFeature();
    }
  });
});
