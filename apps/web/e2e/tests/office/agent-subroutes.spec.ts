import { test, expect } from "../../fixtures/office-fixture";

/**
 * Agent detail sub-route coverage. Wave 1 of the
 * office-agent-detail-overhaul refactor split the old single-page
 * tab strip into real bookmarkable URLs at
 * `/office/agents/[id]/<segment>/page.tsx`. This spec pins:
 *
 *   - Every segment renders without crashing.
 *   - The URL stays put (no client-side redirect to dashboard).
 *   - The shared layout's tab strip highlights the active tab.
 *   - The bare `/office/agents/:id` URL redirects to `/dashboard`.
 *   - Clicking a tab keeps the layout's header card mounted (i.e.
 *     navigation is layout-shared, not a full re-render).
 */

const SEGMENTS = [
  "dashboard",
  "permissions",
  "instructions",
  "skills",
  "runs",
  "memory",
  "channels",
] as const;

test.describe("Agent detail sub-routes", () => {
  for (const segment of SEGMENTS) {
    test(`/${segment} renders, URL is bookmarkable, tab is active`, async ({
      testPage,
      officeSeed,
    }) => {
      await testPage.goto(`/office/agents/${officeSeed.agentId}/${segment}`);

      // The active tab gets `border-foreground` + `text-foreground`;
      // inactive tabs use `border-transparent text-muted-foreground
      // hover:text-foreground`. See agents/[id]/layout.tsx.
      const activeTab = testPage.getByTestId(`agent-tab-${segment}`);
      await expect(activeTab).toBeVisible();
      await expect(activeTab).toHaveClass(/border-foreground/);

      // The body region rendered by the layout's <div data-testid="agent-detail-section">.
      await expect(testPage.getByTestId("agent-detail-section")).toBeVisible();

      // URL stays on this segment — no client-side redirect bouncing
      // it back to dashboard or anywhere else.
      expect(testPage.url()).toContain(`/office/agents/${officeSeed.agentId}/${segment}`);
    });
  }

  test("bare /office/agents/:id redirects to /dashboard", async ({ testPage, officeSeed }) => {
    await testPage.goto(`/office/agents/${officeSeed.agentId}`);
    await testPage.waitForURL(new RegExp(`/office/agents/${officeSeed.agentId}/dashboard$`));
  });

  test("clicking a tab in the strip navigates without losing the agent header", async ({
    testPage,
    officeSeed,
  }) => {
    await testPage.goto(`/office/agents/${officeSeed.agentId}/dashboard`);

    // The agent header card is rendered by the shared layout — confirm
    // it's there before the click so the post-click assertion is meaningful.
    const main = testPage.locator("main");
    await expect(main.getByText("CEO", { exact: true }).first()).toBeVisible({
      timeout: 10_000,
    });

    await testPage.getByTestId("agent-tab-runs").click();
    await testPage.waitForURL(/\/runs$/);

    // Header card stays — confirms the layout is shared, not unmounted
    // per-route. If the layout gets re-rendered this would still pass,
    // but if a future refactor accidentally moves the header into a
    // segment page it would fail.
    await expect(main.getByText("CEO", { exact: true }).first()).toBeVisible();

    // The runs tab is now active (uses `border-foreground`).
    await expect(testPage.getByTestId("agent-tab-runs")).toHaveClass(/border-foreground/);
    // The dashboard tab is no longer active. Inactive tabs use
    // `border-transparent`.
    await expect(testPage.getByTestId("agent-tab-dashboard")).toHaveClass(/border-transparent/);
  });
});
