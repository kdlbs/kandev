// Plan panel search — TipTap PlanSearchExtension integration.
import { test, expect } from "../../fixtures/test-base";
import {
  openPanelSearch,
  panelSearchBar,
  panelSearchInput,
  panelSearchMatchCounter,
  panelSearchToggle,
  closePanelSearch,
} from "../../helpers/panel-search";
import { seedTask, planScript } from "./shared";

const PLAN_CONTENT = [
  "## Overview",
  "The quick brown fox jumps over the lazy dog.",
  "",
  "## Details",
  "Another fox reference. Quick sandboxed plan section.",
  "",
  "## Conclusion",
  "No FOXES here.",
].join("\n");

async function openPlanPanel(
  page: import("@playwright/test").Page,
  session: import("../../pages/session-page").SessionPage,
): Promise<void> {
  await session.togglePlanMode();
  await expect(page.getByTestId("plan-panel")).toBeVisible({ timeout: 15_000 });
  await expect(page.getByTestId("plan-panel").locator(".ProseMirror")).toBeVisible({
    timeout: 15_000,
  });
  await expect(
    page.getByTestId("plan-panel").getByText("quick brown fox", { exact: false }),
  ).toBeVisible({ timeout: 15_000 });
}

test.describe("@search plan panel search", () => {
  test.describe.configure({ retries: 1 });

  test("P1+P2+P3 highlights all matches with exactly one current, counter reflects count", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "plan-search-basic", {
      description: planScript(PLAN_CONTENT),
    });
    await openPlanPanel(testPage, session);

    await openPanelSearch(testPage, "plan");
    await prCapture.startRecording("plan-search");
    await panelSearchInput(testPage).fill("fox");

    // Wait for decorations to be applied (plugin runs on transaction)
    const highlights = testPage.locator(".ProseMirror .search-highlight");
    await expect(highlights.first()).toBeVisible({ timeout: 5_000 });
    expect(await highlights.count()).toBeGreaterThanOrEqual(2);
    // Exactly one current-match
    const current = testPage.locator(".ProseMirror .search-highlight-current");
    await expect(current).toHaveCount(1);
    // Counter reflects the plugin state
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 5_000,
      })
      .toMatch(/^1 \/ [2-9]\d*$/);
    await prCapture.stopRecording({ caption: "Plan search: next/prev match navigation" });
  });

  test("P4+P5+P6 Enter advances, Shift+Enter wraps backward", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "plan-search-nav", {
      description: planScript(PLAN_CONTENT),
    });
    await openPlanPanel(testPage, session);

    await openPanelSearch(testPage, "plan");
    await panelSearchInput(testPage).fill("fox");
    // Wait for counter to stabilize
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 5_000,
      })
      .toMatch(/^1 \/ /);

    const total = Number(
      (await panelSearchMatchCounter(testPage).innerText()).trim().split("/")[1].trim(),
    );
    expect(total).toBeGreaterThanOrEqual(2);

    await panelSearchInput(testPage).press("Enter");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 3_000,
      })
      .toBe(`2 / ${total}`);

    // Shift+Enter at 2/N goes back to 1/N
    await panelSearchInput(testPage).press("Shift+Enter");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 3_000,
      })
      .toBe(`1 / ${total}`);

    // Shift+Enter wraps from 1/N to N/N
    await panelSearchInput(testPage).press("Shift+Enter");
    await expect
      .poll(async () => (await panelSearchMatchCounter(testPage).innerText()).trim(), {
        timeout: 3_000,
      })
      .toBe(`${total} / ${total}`);
  });

  test("P9 Close removes all highlights", async ({ testPage, apiClient, seedData }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "plan-search-close", {
      description: planScript(PLAN_CONTENT),
    });
    await openPlanPanel(testPage, session);

    await openPanelSearch(testPage, "plan");
    await panelSearchInput(testPage).fill("fox");
    await expect(testPage.locator(".ProseMirror .search-highlight").first()).toBeVisible({
      timeout: 5_000,
    });

    await closePanelSearch(testPage);
    await expect(testPage.locator(".ProseMirror .search-highlight")).toHaveCount(0, {
      timeout: 3_000,
    });
    await expect(testPage.locator(".ProseMirror .search-highlight-current")).toHaveCount(0);
  });

  test("P10 plan bar does not expose case/regex toggles", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const { session } = await seedTask(testPage, apiClient, seedData, "plan-search-toggles", {
      description: planScript(PLAN_CONTENT),
    });
    await openPlanPanel(testPage, session);

    await openPanelSearch(testPage, "plan");
    await expect(panelSearchToggle(testPage, "Match case")).toHaveCount(0);
    await expect(panelSearchToggle(testPage, "Regular expression")).toHaveCount(0);
    // But core elements still there
    await expect(panelSearchInput(testPage)).toBeVisible();
    await expect(panelSearchMatchCounter(testPage)).toBeVisible();
    // Bar remains visible (smoke-check)
    await expect(panelSearchBar(testPage)).toBeVisible();
  });

  // @covers AC-UI-PLAN-EDITOR-TASK-SWITCH-001.1
  // @covers AC-UI-PLAN-EDITOR-TASK-SWITCH-001.4
  test("switches sidebar tasks while their Plan panels are open", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    test.setTimeout(180_000);
    const pageErrors: string[] = [];
    const routeErrors: string[] = [];
    testPage.on("pageerror", (error) => pageErrors.push(error.message));
    testPage.on("console", (message) => {
      if (message.type() === "error" && /\[app\] route render failed/i.test(message.text())) {
        routeErrors.push(message.text());
      }
    });

    const first = await seedTask(testPage, apiClient, seedData, "Plan switch alpha", {
      description: planScript(PLAN_CONTENT.replace("sandboxed", "alpha")),
    });
    await openPlanPanel(testPage, first.session);

    const second = await seedTask(testPage, apiClient, seedData, "Plan switch beta", {
      description: planScript(PLAN_CONTENT.replace("sandboxed", "beta")),
    });
    await openPlanPanel(testPage, second.session);

    await second.session.sidebarTaskItem("Plan switch alpha").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${first.taskId}$`));
    await expect(testPage.getByTestId("plan-panel")).toContainText("alpha plan section");

    await second.session.sidebarTaskItem("Plan switch beta").click();
    await expect(testPage).toHaveURL(new RegExp(`/t/${second.taskId}$`));
    await expect(testPage.getByTestId("plan-panel")).toContainText("beta plan section");
    await expect(
      testPage.getByRole("alert").filter({ hasText: "This page couldn’t load." }),
    ).toHaveCount(0);

    await openPanelSearch(testPage, "plan");
    await panelSearchInput(testPage).fill("fox");
    await expect(testPage.locator(".ProseMirror .search-highlight").first()).toBeVisible();
    await prCapture.screenshot("plan-task-switch-stable", {
      caption: "Plan search remains available after switching sidebar tasks",
    });

    expect(pageErrors, JSON.stringify(pageErrors, null, 2)).toEqual([]);
    expect(routeErrors, JSON.stringify(routeErrors, null, 2)).toEqual([]);
  });
});
