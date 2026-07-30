import type { Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

type E2EModelStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    setState: (
      updater: (state: {
        sessionModels: {
          bySessionId: Record<
            string,
            {
              currentModelId: string;
              models: unknown[];
              configOptions: {
                id: string;
                options?: { value: string; name: string; description?: string }[];
              }[];
            }
          >;
        };
      }) => void,
    ) => void;
  };
};

async function seedAndOpenTask(testPage: Page, apiClient: ApiClient, seedData: SeedData) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Model Selector",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { session, task };
}

async function seedLongReasoningMenu(testPage: Page, sessionId: string) {
  await testPage.evaluate((sid) => {
    const store = (window as E2EModelStoreWindow).__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge is unavailable");
    store.setState((state) => {
      const sessionModels = state.sessionModels.bySessionId[sid];
      if (!sessionModels) throw new Error(`Session ${sid} has no model state`);
      sessionModels.configOptions = sessionModels.configOptions.map((option) =>
        option.id === "effort"
          ? {
              ...option,
              options: Array.from({ length: 10 }, (_, index) => ({
                value: index === 0 ? "low" : `effort-${index + 1}`,
                name: index === 0 ? "Low" : `Reasoning level ${index + 1}`,
                description:
                  index === 0
                    ? "Faster responses with less reasoning"
                    : `Reasoning depth description for level ${index + 1}`,
              })),
            }
          : option,
      );
    });
  }, sessionId);
}

test.describe("Mobile chat model selector", () => {
  test.describe.configure({ retries: 1, timeout: 60_000 });

  test("shows compact changes and provider descriptions by touch", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const { task } = await seedAndOpenTask(testPage, apiClient, seedData);
    expect(task.session_id).toBeTruthy();

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const baseline = sessions[0]?.metadata?.acp_config_baseline as
          | Record<string, string>
          | undefined;
        return baseline?.effort;
      })
      .toBe("medium");

    await seedLongReasoningMenu(testPage, task.session_id!);

    const leftActions = testPage.getByTestId("mobile-chat-toolbar-left-actions");
    await expect(leftActions).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("toolbar-overflow-menu")).not.toBeVisible();
    await expect(leftActions.getByTestId("toolbar-item-sessions")).toHaveCount(0);

    await expect(leftActions.getByTestId("session-mode-selector")).toBeVisible();

    const trigger = leftActions.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toHaveText("Mock Fast", { timeout: 15_000 });

    await trigger.tap();
    await expect(testPage.getByRole("option", { name: /Mock Smart/ })).toBeVisible({
      timeout: 5_000,
    });
    await expect(
      testPage.getByRole("option", { name: /Mock Fast/ }).getByTitle("Fast mock model for testing"),
    ).toBeVisible();
    await expect(
      testPage
        .getByRole("option", { name: /Mock Smart/ })
        .getByTitle("Smart mock model for testing"),
    ).toBeVisible();

    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible();
    await expect(
      testPage.getByText("Controls how much reasoning the mock model uses", { exact: true }),
    ).toHaveCount(0);
    await effortTrigger.tap();
    const effortSection = testPage.getByTestId("config-option-section-effort");
    await expect(effortSection).toBeVisible();
    const overflow = await effortSection.evaluate((element) => ({
      clientHeight: element.clientHeight,
      overflowY: getComputedStyle(element).overflowY,
      scrollHeight: element.scrollHeight,
    }));
    expect(overflow.overflowY).toBe("auto");
    expect(overflow.scrollHeight).toBeGreaterThan(overflow.clientHeight);
    const effortBox = await effortSection.boundingBox();
    expect(effortBox).not.toBeNull();
    await testPage.mouse.move(
      effortBox!.x + effortBox!.width / 2,
      effortBox!.y + effortBox!.height / 2,
    );
    await testPage.mouse.wheel(0, 800);
    await expect
      .poll(() => effortSection.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(0);
    await testInfo.attach("task-model-reasoning-scroll-mobile", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });
    await expect(
      testPage.getByText("Controls how much reasoning the mock model uses", { exact: true }),
    ).toBeVisible();
    await expect(
      testPage.getByText("Faster responses with less reasoning", { exact: true }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: "Low", exact: true }).tap();
    await expect(trigger).toHaveText("Mock Fast / Low");
    await expect(trigger).not.toContainText("Medium");

    await expect(
      testPage.getByRole("option", { name: /Mock Fast/ }).getByTitle("Fast mock model for testing"),
    ).toBeVisible();
    await testInfo.attach("task-model-selector-mobile", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });
    await prCapture.screenshot("mobile-model-selector", {
      caption: "Mobile model selector remains reachable by touch",
    });

    const viewport = testPage.viewportSize();
    expect(viewport).not.toBeNull();
    const popoverBox = await testPage.getByRole("option", { name: /Mock Smart/ }).boundingBox();
    expect(popoverBox).not.toBeNull();
    expect(popoverBox!.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox!.x + popoverBox!.width).toBeLessThanOrEqual(viewport!.width);
  });
});
