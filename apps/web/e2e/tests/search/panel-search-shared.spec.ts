// Panel-scoped Ctrl+F search — cross-panel behaviours.
// Asserts the shared contract (open/close/focus-scoping) against all three
// panel kinds using the same test body.
import { test, expect } from "../../fixtures/test-base";
import {
  openPanelSearch,
  closePanelSearch,
  panelSearchBar,
  panelSearchInput,
  panelSearchMatchCounter,
  type PanelKind,
} from "../../helpers/panel-search";
import { seedTask, seedMessagesDescription, planScript } from "./shared";
import { SessionPage } from "../../pages/session-page";

const panels: PanelKind[] = ["session", "plan", "terminal"];

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";

test.describe("@search panel search bar — shared contract", () => {
  test.describe.configure({ retries: 1 });

  for (const kind of panels) {
    test.describe(`panel=${kind}`, () => {
      test(`S1+S7 Ctrl+F opens bar, autofocuses input, shows counter`, async ({
        testPage,
        apiClient,
        seedData,
      }) => {
        test.setTimeout(90_000);
        const description =
          kind === "plan"
            ? planScript("## Scratch\nnothing special here")
            : seedMessagesDescription(["hello world alpha", "hello world beta"]);
        const { session } = await seedTask(
          testPage,
          apiClient,
          seedData,
          `shared-${kind}-S1`,
          description,
        );
        await preparePanel(session, kind, testPage);

        await openPanelSearch(testPage, kind);

        await expect(panelSearchInput(testPage)).toBeFocused();
        await expect(panelSearchInput(testPage)).toHaveValue("");
        // Counter is always mounted (may show 0 / 0 with no query)
        await expect(panelSearchMatchCounter(testPage)).toBeVisible();
      });

      test(`S3 Esc closes the bar`, async ({ testPage, apiClient, seedData }) => {
        test.setTimeout(90_000);
        const description =
          kind === "plan" ? planScript("## Scratch\nnothing") : seedMessagesDescription(["hello"]);
        const { session } = await seedTask(
          testPage,
          apiClient,
          seedData,
          `shared-${kind}-S3`,
          description,
        );
        await preparePanel(session, kind, testPage);

        await openPanelSearch(testPage, kind);
        await closePanelSearch(testPage);
        await expect(panelSearchBar(testPage)).toHaveCount(0);
      });

      test(`S4 Close button closes the bar`, async ({ testPage, apiClient, seedData }) => {
        test.setTimeout(90_000);
        const description =
          kind === "plan" ? planScript("## Scratch\ntext") : seedMessagesDescription(["foo"]);
        const { session } = await seedTask(
          testPage,
          apiClient,
          seedData,
          `shared-${kind}-S4`,
          description,
        );
        await preparePanel(session, kind, testPage);

        await openPanelSearch(testPage, kind);
        // Button by title="Close (Esc)"
        const closeBtn = panelSearchBar(testPage).getByRole("button", { name: /Close/ });
        await closeBtn.click();
        await expect(panelSearchBar(testPage)).toHaveCount(0);
      });

      test(`S5 Reopening restores empty query`, async ({ testPage, apiClient, seedData }) => {
        test.setTimeout(90_000);
        const description =
          kind === "plan"
            ? planScript("## Sample\nAlphaBetaGamma")
            : seedMessagesDescription(["AlphaBetaGamma"]);
        const { session } = await seedTask(
          testPage,
          apiClient,
          seedData,
          `shared-${kind}-S5`,
          description,
        );
        await preparePanel(session, kind, testPage);

        await openPanelSearch(testPage, kind);
        await panelSearchInput(testPage).fill("Alpha");
        await expect(panelSearchInput(testPage)).toHaveValue("Alpha");
        await closePanelSearch(testPage);

        await openPanelSearch(testPage, kind);
        await expect(panelSearchInput(testPage)).toHaveValue("");
      });
    });
  }
});

/**
 * Ensure the given panel is rendered, visible, and ready to receive keyboard focus.
 * For the terminal panel we additionally wait for the xterm buffer to have some
 * content (indicates the shell has connected).
 */
async function preparePanel(
  session: SessionPage,
  kind: PanelKind,
  page: import("@playwright/test").Page,
): Promise<void> {
  if (kind === "session") {
    await expect(session.chat).toBeVisible({ timeout: 10_000 });
    return;
  }
  if (kind === "plan") {
    // Default layout doesn't include the plan panel — enable plan mode to add it.
    await session.togglePlanMode();
    await expect(session.planPanel).toBeVisible({ timeout: 15_000 });
    // Wait for seeded plan content to render
    await expect(session.planPanel.locator(".ProseMirror")).toBeVisible({ timeout: 15_000 });
    return;
  }
  // terminal
  await expect(session.terminal).toBeVisible({ timeout: 15_000 });
  await expect
    .poll(
      async () =>
        page.evaluate(() => {
          const panel = document.querySelector('[data-testid="terminal-panel"]');
          const xtermEl = panel?.querySelector(".xterm");
          type XC = HTMLElement & { __xtermReadBuffer?: () => string };
          const container = xtermEl?.parentElement as XC | null | undefined;
          return (container?.__xtermReadBuffer?.() ?? "").length > 0;
        }),
      { timeout: 20_000, message: "Waiting for terminal shell buffer" },
    )
    .toBe(true);
}

void MODIFIER;
