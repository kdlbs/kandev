import type { AppState } from "../../../lib/state/store";
import { test, expect } from "../../fixtures/test-base";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

// @covers AC-UI-COMPOSER-PROVIDER-ICON-001.1, .2, .3
for (const failLogo of [false, true]) {
  test(`composer provider appears in trigger and popover (logo failure: ${failLogo})`, async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const ws = watchWs(testPage);
    await testPage.addInitScript(
      (theme) => localStorage.setItem("theme", theme),
      failLogo ? "light" : "dark",
    );
    await testPage.route("**/api/v1/agents/*/logo?*", (route) =>
      failLogo
        ? route.fulfill({ status: 404 })
        : route.fulfill({
            contentType: "image/svg+xml",
            path: "../backend/internal/agent/agents/logos/codex_dark.svg",
          }),
    );
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Composer provider icon",
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
    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    const icon = trigger.getByTestId("model-provider-icon");
    await expect(icon.locator(failLogo ? "svg" : "img")).toBeVisible();
    await trigger.focus();
    await testPage.keyboard.press("Enter");
    const group = testPage.getByRole("group", { name: "Model", exact: true });
    await expect(
      testPage.locator("[cmdk-group-heading]").getByTestId("model-provider-icon"),
    ).toBeVisible();
    await expect(group.getByRole("option", { name: /Mock Smart/ })).toBeVisible();
    await testPage.screenshot({ path: testInfo.outputPath("provider-picker.png") });
    await prCapture.screenshot(`composer-provider-${failLogo ? "fallback" : "logo"}`, {
      caption: "CLI identity in the composer and open model picker",
    });
    const updated = ws.waitForEvent("session.models_updated", {
      where: (payload) =>
        payload.session_id === task.session_id && payload.current_model_id === "mock-smart",
    });
    await group.getByRole("option", { name: /Mock Smart/ }).click();
    await updated;
    await expect(trigger).toContainText("Mock Smart");
    await expect(icon).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByRole("listbox")).toBeHidden();
    await testPage.evaluate((sessionId) => {
      const store = (
        window as unknown as {
          __KANDEV_E2E_STORE__: {
            getState: () => AppState;
            setState: (state: Partial<AppState>) => void;
          };
        }
      ).__KANDEV_E2E_STORE__;
      const state = store.getState();
      const entry = state.sessionModels.bySessionId[sessionId!];
      const name =
        "A very long model name that should truncate before pushing the provider logo out of view";
      store.setState({
        sessionModels: {
          bySessionId: {
            ...state.sessionModels.bySessionId,
            [sessionId!]: {
              ...entry,
              models: entry.models.map((model) => ({ ...model, name })),
              configOptions: entry.configOptions.map((option) => ({
                ...option,
                options: option.options?.map((value) => ({ ...value, name })),
              })),
            },
          },
        },
      });
    }, task.session_id);
    await expect(trigger).toContainText("A very long model name");
    const bounds = await icon.boundingBox();
    const triggerBounds = (await trigger.boundingBox())!;
    expect(bounds!.x).toBeGreaterThanOrEqual(triggerBounds.x);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(triggerBounds.x + triggerBounds.width);
    expect(bounds?.width).toBe(14);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  });
}
