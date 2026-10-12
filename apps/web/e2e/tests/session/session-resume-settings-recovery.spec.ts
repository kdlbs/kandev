import { test } from "../../fixtures/test-base";
import { runSessionResumeSettingsRecoveryE2E } from "../../helpers/session-resume-settings-recovery";

test.describe("desktop: Auggie session settings recovery", () => {
  test.describe.configure({ retries: 0 });

  let profileId: string | undefined;
  let taskId: string | undefined;
  let mockAuggieEnabled = false;

  test.afterEach(async ({ apiClient, backend, seedData }) => {
    if (taskId) await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
    if (mockAuggieEnabled) await backend.restart();
    profileId = undefined;
    taskId = undefined;
    mockAuggieEnabled = false;
  });

  test("explains one-attempt settings omission and resumes the same ACP conversation", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }, testInfo) => {
    test.setTimeout(180_000);
    await runSessionResumeSettingsRecoveryE2E({
      page: testPage,
      apiClient,
      seedData,
      backend,
      testInfo,
      mobile: false,
      prCapture,
      onMockAuggieEnabled: () => {
        mockAuggieEnabled = true;
      },
      onProfileCreated: (id) => {
        profileId = id;
      },
      onTaskCreated: (id) => {
        taskId = id;
      },
    });
  });
});
