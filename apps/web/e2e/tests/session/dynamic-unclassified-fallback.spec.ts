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
import { waitForAgentMessage, waitForSessionDone } from "../../helpers/session";

test.describe("dynamic unclassified fallback", () => {
  test("uses repeated safe failures and keeps unsafe cases on manual recovery", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(360_000);
    const releaseFeature = await backend.useEnv({
      KANDEV_FEATURES_DYNAMIC_AGENT_ROUTING: "true",
    });

    try {
      const enabled = await createDynamicFallbackProfile(apiClient, seedData, {
        name: "Unclassified desktop core",
        enabled: true,
      });
      const savedPolicy = enabled.dynamicProfile.dynamic?.candidates[0]?.policies;
      if (!savedPolicy) throw new Error("Dynamic profile did not return its candidate policy");
      await apiClient.updateAgentProfile(enabled.dynamicProfile.id, {
        name: "Unclassified desktop core renamed",
      });
      const editedProfile = await apiClient.getAgentProfile(enabled.dynamicProfile.id);
      expect(editedProfile.dynamic?.candidates[0]?.policies).toEqual(savedPolicy);

      const core = await startDynamicFallbackSession(testPage, apiClient, seedData, {
        title: "Unclassified desktop core",
        prompt: "/e2e:dynamic-unclassified:same:3",
        profileId: enabled.dynamicProfile.id,
      });
      const firstFailure = await waitForRouteActionRequired(apiClient, core.taskId, core.sessionId);
      await expect(core.session.activeChat().getByTestId("dynamic-route-retry")).toBeVisible();
      await expectCurrentCandidate(
        apiClient,
        core.taskId,
        core.sessionId,
        enabled.dynamicProfile.id,
        enabled.firstCandidate.id,
      );

      const secondFailure = await retryCurrentDynamicCandidate({
        page: testPage,
        apiClient,
        taskId: core.taskId,
        sessionId: core.sessionId,
      });
      expect(secondFailure.execution_profile_id).toBe(enabled.firstCandidate.id);

      const retryAfterSecondFailure = testPage.getByTestId("dynamic-route-retry");
      await expect(retryAfterSecondFailure).toBeVisible({ timeout: 10_000 });
      await expect(retryAfterSecondFailure).toBeEnabled({ timeout: 10_000 });
      await retryAfterSecondFailure.click({ timeout: 10_000 });
      await waitForAgentMessage(apiClient, core.sessionId, DYNAMIC_FALLBACK_SUCCESS, 120_000);
      await waitForSessionDone(
        apiClient,
        core.taskId,
        core.sessionId,
        "successor response settled",
      );
      await expect(
        core.session.activeChat().getByText(DYNAMIC_FALLBACK_SUCCESS, { exact: true }),
      ).toBeVisible();

      const draft = core.session
        .activeChat()
        .locator('.tiptap.ProseMirror[contenteditable="true"]')
        .first();
      await expect(draft).toBeEditable();
      await draft.fill(DYNAMIC_FALLBACK_DRAFT);
      await expect(draft).toHaveText(DYNAMIC_FALLBACK_DRAFT);

      const afterSwitch = await expectCurrentCandidate(
        apiClient,
        core.taskId,
        core.sessionId,
        enabled.dynamicProfile.id,
        enabled.secondCandidate.id,
      );
      expect(afterSwitch?.route_generation).toBeGreaterThan(firstFailure.route_generation ?? 0);
      const sessionBeforeReload = afterSwitch;
      await testPage.reload();
      await core.session.waitForLoad();
      await expect(
        core.session.activeChat().getByText(DYNAMIC_FALLBACK_SUCCESS, { exact: true }),
      ).toBeVisible();
      const reloadedDraft = core.session
        .activeChat()
        .locator('.tiptap.ProseMirror[contenteditable="true"]');
      await expect(reloadedDraft).toBeEditable();
      await expect(reloadedDraft).toHaveText(DYNAMIC_FALLBACK_DRAFT);
      const afterReload = await expectCurrentCandidate(
        apiClient,
        core.taskId,
        core.sessionId,
        enabled.dynamicProfile.id,
        enabled.secondCandidate.id,
      );
      expect(afterReload?.route_generation).toBe(sessionBeforeReload?.route_generation);
      const { messages } = await apiClient.listSessionMessages(core.sessionId);
      expect(
        messages.filter((message) => message.content === DYNAMIC_FALLBACK_SUCCESS),
      ).toHaveLength(1);

      const runManualCase = async (options: {
        suffix: string;
        prompt: string;
        enabled: boolean;
        manualRetries?: number;
        veto?: boolean;
      }) => {
        const profile = await createDynamicFallbackProfile(apiClient, seedData, {
          name: `Unclassified desktop ${options.suffix}`,
          enabled: options.enabled,
        });
        if (options.veto) {
          await apiClient.updateWorkflowStep(seedData.startStepId, {
            disable_unclassified_fallback: true,
          });
        }
        const attempt = await startDynamicFallbackSession(testPage, apiClient, seedData, {
          title: `Unclassified desktop ${options.suffix}`,
          prompt: options.prompt,
          profileId: profile.dynamicProfile.id,
        });
        await waitForRouteActionRequired(apiClient, attempt.taskId, attempt.sessionId);
        for (let retry = 0; retry < (options.manualRetries ?? 0); retry += 1) {
          await retryCurrentDynamicCandidate({
            page: testPage,
            apiClient,
            taskId: attempt.taskId,
            sessionId: attempt.sessionId,
          });
        }
        await expect(attempt.session.activeChat().getByTestId("dynamic-route-retry")).toBeVisible();
        await expectCurrentCandidate(
          apiClient,
          attempt.taskId,
          attempt.sessionId,
          profile.dynamicProfile.id,
          profile.firstCandidate.id,
        );
        if (options.veto) {
          await apiClient.updateWorkflowStep(seedData.startStepId, {
            disable_unclassified_fallback: false,
          });
        }
      };

      await runManualCase({
        suffix: "off",
        prompt: "/e2e:dynamic-unclassified:same:3",
        enabled: false,
      });
      await runManualCase({
        suffix: "step-veto",
        prompt: "/e2e:dynamic-unclassified:same:3",
        enabled: true,
        veto: true,
      });
      await runManualCase({
        suffix: "different-diagnostics",
        prompt: "/e2e:dynamic-unclassified:mixed:3",
        enabled: true,
        manualRetries: 2,
      });
      await runManualCase({
        suffix: "output-evidence",
        prompt: "/e2e:dynamic-unclassified:output:3",
        enabled: true,
      });
      await runManualCase({
        suffix: "tool-effect",
        prompt: "/e2e:dynamic-unclassified:tool:3",
        enabled: true,
      });
    } finally {
      await apiClient.updateWorkflowStep(seedData.startStepId, {
        disable_unclassified_fallback: false,
      });
      await releaseFeature();
    }
  });
});
