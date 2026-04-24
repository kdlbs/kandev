// Panel-scoped Ctrl+F — focus routing between panels.
import { test, expect } from "../../fixtures/test-base";
import { openPanelSearch, panelSearchBar } from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription, planScript } from "./shared";

test.describe("@search panel-focus routing", () => {
  test.describe.configure({ retries: 1 });

  test("F1 chat-focused Ctrl+F opens a bar inside the session panel only", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(
      testPage,
      apiClient,
      seedData,
      "focus-routing-session",
      seedMessagesDescription(["alpha"]),
    );
    await expect(session.chat.getByText("alpha", { exact: false }).first()).toBeVisible({
      timeout: 30_000,
    });

    await openPanelSearch(testPage, "session");

    // The bar is mounted inside the session-chat panel (descendant)
    const inSession = session.chat.locator("[data-panel-search-bar]");
    await expect(inSession).toBeVisible();
    expect(await panelSearchBar(testPage).count()).toBe(1);
  });

  test("F3 terminal-focused Ctrl+F opens the bar inside the terminal only", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    await seedTask(
      testPage,
      apiClient,
      seedData,
      "focus-routing-terminal",
      seedMessagesDescription(["alpha"]),
    );

    // Ensure terminal panel is mounted
    await expect(testPage.getByTestId("terminal-panel")).toBeVisible({ timeout: 15_000 });
    await openPanelSearch(testPage, "terminal");

    const inTerminal = testPage.getByTestId("terminal-panel").locator("[data-panel-search-bar]");
    await expect(inTerminal).toBeVisible();
    // Chat panel has none
    expect(
      await testPage.getByTestId("session-chat").locator("[data-panel-search-bar]").count(),
    ).toBe(0);
  });

  test("F2 plan-focused Ctrl+F opens the bar inside the plan panel only", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(
      testPage,
      apiClient,
      seedData,
      "focus-routing-plan",
      planScript("## Plan\nsome content here"),
    );
    await session.togglePlanMode();
    await expect(session.planPanel.locator(".ProseMirror")).toBeVisible({ timeout: 15_000 });

    await openPanelSearch(testPage, "plan");
    const inPlan = session.planPanel.locator("[data-panel-search-bar]");
    await expect(inPlan).toBeVisible();
    // Chat panel has none
    expect(await session.chat.locator("[data-panel-search-bar]").count()).toBe(0);
  });
});
