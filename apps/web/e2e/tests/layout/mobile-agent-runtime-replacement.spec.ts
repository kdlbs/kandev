import { expect, test } from "../../fixtures/test-base";
import {
  observeAgentRuntimeAvailability,
  waitForAgentRuntimeReplacement,
} from "../../helpers/agent-runtime-availability";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

type RuntimeStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState: () => {
      agentRuntime: {
        status: string;
        boot_id?: string;
        runtime_epoch?: number;
      } | null;
    };
  };
};

test.describe("Mobile agent runtime replacement", () => {
  test.describe.configure({ retries: 1 });

  test("keeps the task usable after a real local runtime replacement", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile agent runtime replacement keeps this task open",
      seedData.agentProfileId,
      {
        description: "/slow 30",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(session.chat.getByText("Running slow response", { exact: false })).toBeVisible({
      timeout: 30_000,
    });

    const bootID = await apiClient.getBackendBootID();
    const runtime = await testPage.evaluate(
      () => (window as RuntimeStoreWindow).__KANDEV_E2E_STORE__?.getState().agentRuntime ?? null,
    );
    if (!runtime || runtime.status !== "available" || runtime.runtime_epoch === undefined) {
      throw new Error("local runtime is not available before the child-death test");
    }
    const pageTimeOrigin = await testPage.evaluate(() => performance.timeOrigin);
    await observeAgentRuntimeAvailability(testPage);

    const killed = await apiClient.killLocalAgentRuntimeChild();
    expect(killed.killed).toBe(true);
    await waitForAgentRuntimeReplacement(testPage, runtime.runtime_epoch, bootID);

    expect(await apiClient.getBackendBootID()).toBe(bootID);
    expect(await testPage.evaluate(() => performance.timeOrigin)).toBe(pageTimeOrigin);
    await expect(testPage).toHaveURL(new RegExp(`/t/${task.id}$`));
    await expect(testPage.getByTestId("agent-runtime-alert")).toHaveCount(0);
    await expect(
      session.chat.getByText("Delivery was interrupted. The prompt outcome is uncertain.", {
        exact: true,
      }),
    ).toBeVisible({ timeout: 30_000 });

    const stop = session.chat.getByTestId("recovery-stop-button");
    await expect(stop).toBeEnabled();
    const stopBox = await stop.boundingBox();
    expect(stopBox?.height ?? 0).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile runtime replacement");
  });
});
