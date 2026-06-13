import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page } from "@playwright/test";

/**
 * Seed a task using the diff-expansion-setup mock scenario and navigate to
 * its session page, waiting for the agent turn to complete.
 *
 * The scenario writes a 50-line file, commits it, then modifies two lines far
 * apart (line 3 and line 48).  The diff viewer will show two separate hunks
 * with ~44 collapsed lines between them.
 */
async function seedExpansionTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Diff Expansion E2E",
    seedData.agentProfileId,
    {
      description: "/e2e:diff-expansion-setup",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

  await testPage.goto(`/t/${task.id}`);

  const session = new SessionPage(testPage);
  await session.waitForLoad();

  await expect(
    session.chat.getByText("diff-expansion-setup complete", { exact: false }),
  ).toBeVisible({ timeout: 45_000 });

  return session;
}

/** Click the Changes dockview tab. */
async function openChangesTab(testPage: Page) {
  const changesTab = testPage.locator(".dv-default-tab", { hasText: "Changes" });
  await expect(changesTab).toBeVisible({ timeout: 10_000 });
  await changesTab.click();
}

/** Click the file row for expansion_test.go to open its diff view. */
async function openExpansionFileDiff(testPage: Page) {
  const fileRow = testPage.getByTestId("file-row-expansion_test.go");
  await expect(fileRow).toBeVisible({ timeout: 10_000 });
  await fileRow.click();
}

async function waitForDiffText(testPage: Page, text: string, timeout = 60_000) {
  await expect
    .poll(
      () =>
        testPage.evaluate((expected) => {
          for (const container of document.querySelectorAll("diffs-container")) {
            if (container.shadowRoot?.textContent?.includes(expected)) return true;
          }
          return false;
        }, text),
      { timeout },
    )
    .toBe(true);
}

async function hoverUntilGutterSlotAppears(testPage: Page) {
  const points = await testPage.evaluate(() => {
    const container = document.querySelector("diffs-container");
    const shadow = container?.shadowRoot;
    if (!shadow) throw new Error("diffs-container shadow root missing");
    const line = shadow.querySelector<HTMLElement>("[data-line]");
    if (!line) throw new Error("no [data-line] found to hover");
    const r = line.getBoundingClientRect();
    const y = r.top + r.height / 2;
    return [
      { x: r.left + 2, y },
      { x: r.left + 10, y },
      { x: r.left + Math.min(40, r.width / 4), y },
      { x: r.left + r.width / 2, y },
    ];
  });

  for (const point of points) {
    await testPage.mouse.move(point.x, point.y);
    const appeared = await testPage
      .waitForFunction(
        () =>
          Boolean(
            document
              .querySelector("diffs-container")
              ?.shadowRoot?.querySelector("[data-gutter-utility-slot]"),
          ),
        null,
        { timeout: 1_500 },
      )
      .then(() => true)
      .catch(() => false);
    if (appeared) return;
  }

  throw new Error("gutter-utility-slot did not appear after hover");
}

test.describe("Diff expansion — Pierre Diffs provider", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("diff viewer background matches app --background (regression for pierre 1.1.22 selector rename)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });
    await waitForDiffText(testPage, "HUNK_TOP");

    // Pierre's <pre data-diff> uses var(--diffs-bg); our unsafeCSS overrides
    // that variable to var(--background) on :host. If the selector ever stops
    // matching (as happened on the 1.0.11 -> 1.1.22 bump that renamed
    // data-diffs -> data-diff), pierre's dark default (#0a0c10) leaks through.
    const colors = await testPage.evaluate(() => {
      const container = document.querySelector("diffs-container");
      if (!container) throw new Error("diffs-container element not found");
      const shadow = container.shadowRoot;
      if (!shadow) throw new Error("diffs-container shadow root is closed or not yet attached");
      const pre = shadow.querySelector<HTMLElement>("pre[data-diff]");
      if (!pre) throw new Error("pre[data-diff] not found in diffs-container shadow root");
      // Resolve var(--background) to a concrete rgb() via a probe element so we
      // can compare it byte-for-byte to the shadow-DOM pre's computed bg.
      const probe = document.createElement("div");
      probe.style.backgroundColor = `var(--background)`;
      document.body.appendChild(probe);
      const expected = getComputedStyle(probe).backgroundColor;
      probe.remove();
      return {
        pre: getComputedStyle(pre).backgroundColor,
        expected,
      };
    });

    await testPage.screenshot({ path: "test-results/diff-bg-regression.png", fullPage: false });
    expect(colors.pre).toBe(colors.expected);
  });

  test("Add-comment hover button is vertically centered in the line gutter", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await waitForDiffText(testPage, "HUNK_TOP");

    // Pierre 1.1.22 declares [data-gutter-utility-slot] as display:flex with
    // top:0/bottom:0 but no align-items, so a fixed-size hover button pins to
    // the top of the line cell instead of centering on the line number.
    // We override align-items: center in unsafeCSS — verify the rule actually
    // reaches the shadow DOM's adopted stylesheets.
    const slotAlignItems = await testPage.evaluate(() => {
      const container = document.querySelector("diffs-container");
      if (!container?.shadowRoot) throw new Error("diffs-container shadow root missing");

      // Inject a probe with the slot's data attribute into the shadow root and
      // read its computed style — this measures the actual cascade end result
      // without depending on lazy hover-triggered slot creation.
      const probe = document.createElement("div");
      probe.setAttribute("data-gutter-utility-slot", "");
      container.shadowRoot.appendChild(probe);
      const computed = getComputedStyle(probe).alignItems;
      probe.remove();
      return computed;
    });

    await testPage.screenshot({ path: "test-results/diff-hover-button-regression.png" });
    expect(slotAlignItems).toBe("center");
  });

  test("Add-comment hover button extrudes past the line-number cell", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await waitForDiffText(testPage, "HUNK_TOP");

    // Pierre appends the gutter-utility slot wrapper INSIDE the line's
    // numberElement on pointer-move (InteractionManager.js: target.numberElement
    // .appendChild(this.gutterUtilityContainer)). The wrapper is right:0 of
    // that cell, so a button with default 0 margin sits inside the cell and
    // overlaps the line number digits. We compensate with margin-right:
    // calc(1ch - 1lh) on our slotted button — same trick pierre uses on its
    // built-in [data-utility-button] — to push it outside the cell into the
    // code area. Verify the button's right edge ends up past the cell's right.
    await hoverUntilGutterSlotAppears(testPage);

    const geometry = await testPage.evaluate(() => {
      const container = document.querySelector("diffs-container")!;
      const shadow = container.shadowRoot!;
      const slotWrapper = shadow.querySelector<HTMLElement>("[data-gutter-utility-slot]");
      if (!slotWrapper) throw new Error("gutter-utility-slot did not appear after hover");
      const numberCell = slotWrapper.parentElement!;
      // The slot wrapper is appended INTO numberCell; our React-rendered button
      // is projected through the named <slot>. Its computed marginRight must
      // resolve to a negative px value for the extrusion to work.
      const slottedLight = document.querySelector<HTMLElement>('[slot="gutter-utility-slot"]');
      if (!slottedLight) throw new Error("light-DOM [slot=gutter-utility-slot] not found");
      const button = slottedLight.firstElementChild as HTMLElement | null;
      if (!button) throw new Error("no button rendered inside slot");
      const buttonRect = button.getBoundingClientRect();
      const cellRect = numberCell.getBoundingClientRect();
      return {
        marginRight: parseFloat(getComputedStyle(button).marginRight),
        buttonRight: buttonRect.right,
        cellRight: cellRect.right,
      };
    });

    await testPage.screenshot({ path: "test-results/diff-hover-button-extrusion.png" });
    expect(geometry.marginRight).toBeLessThan(0);
    expect(geometry.buttonRight).toBeGreaterThan(geometry.cellRight);
  });

  test("renders Pierre Diffs viewer and shows both hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Pierre Diffs renders a diffs-container custom element with an open shadow DOM.
    // Playwright's getByText auto-pierces shadow DOM and auto-retries.
    await expect(testPage.locator("diffs-container")).toBeVisible({ timeout: 15_000 });
    // On cold CI runners (first test in shard, no V8 code cache), resolveLanguagesAndExecuteTask
    // triggers createJavaScriptRegexEngine() which can take 30-40s to JIT-compile.
    // diffs-container mounts immediately but content appears only after the engine is ready.
    await waitForDiffText(testPage, "HUNK_TOP");
    await waitForDiffText(testPage, "HUNK_BOTTOM", 5_000);

    // Shiki renders each token as a <span style="color: #RRGGBB"> inside the
    // diff's shadow DOM. If the worker pool is broken or the @pierre/diffs ↔
    // Shiki contract changes, lines still render as plain text without inline
    // color styles. Highlighting is async (worker pool), so poll instead of
    // reading once.
    await expect
      .poll(
        () =>
          testPage.evaluate(() => {
            const container = document.querySelector("diffs-container");
            const shadow = container?.shadowRoot;
            if (!shadow) return -1;
            let count = 0;
            for (const span of shadow.querySelectorAll<HTMLElement>("span[style]")) {
              if (/color\s*:/i.test(span.getAttribute("style") ?? "")) count++;
            }
            return count;
          }),
        { timeout: 20_000 },
      )
      .toBeGreaterThan(20);
  });

  test("shows expand separator with unmodified line count between hunks", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await waitForDiffText(testPage, "HUNK_TOP", 15_000);

    // Pierre Diffs renders a separator between hunks showing the hidden line count.
    // The separator contains img elements (chevron arrows) for expanding.
    const middleSeparator = testPage.getByText(/\d+ unmodified lines/).nth(1);
    await expect(middleSeparator).toBeVisible({ timeout: 20_000 });
  });

  test("clicking expand arrow reveals the collapsed middle lines", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    await waitForDiffText(testPage, "HUNK_TOP", 15_000);

    // Wait for expand buttons to appear in the shadow DOM. They load
    // asynchronously after full file content is fetched via WebSocket.
    await testPage.waitForFunction(
      () => {
        const container = document.querySelector("diffs-container");
        const shadow = container?.shadowRoot;
        if (!shadow) return false;
        return shadow.querySelectorAll("[data-expand-button]").length >= 3;
      },
      null,
      { timeout: 20_000 },
    );

    // Click the middle separator's expand-up button to reveal lines from the
    // top of the collapsed gap. The middle separator is the only one without
    // data-separator-first or data-separator-last attributes.
    await testPage.evaluate(() => {
      const container = document.querySelector("diffs-container");
      const shadow = container!.shadowRoot!;
      const sel =
        "[data-separator='line-info']:not([data-separator-first]):not([data-separator-last])";
      const btn = shadow.querySelector<HTMLElement>(`${sel} [data-expand-up]`);
      if (!btn) throw new Error("Middle separator expand-up button not found");
      btn.click();
    });

    // Line 60 is within the first 20 lines revealed by expanding from the top hunk.
    await expect(testPage.getByText("original_060", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });

  test("expand-all button reveals all collapsed lines at once", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedExpansionTask(testPage, apiClient, seedData);
    await openChangesTab(testPage);
    await openExpansionFileDiff(testPage);

    // Wait for both hunks and the separator to be present
    await waitForDiffText(testPage, "HUNK_TOP", 15_000);
    await expect(testPage.getByText(/\d+ unmodified lines/).first()).toBeVisible({
      timeout: 20_000,
    });

    // The Changes tab renders ReviewDiffList → FileDiffToolbar (not the
    // Pierre Diffs renderHeaderMetadata toolbar), and its expand-all button
    // has aria-label "Expand all". Anchor on role+name instead of the
    // Tabler icon class so an icon swap doesn't silently break this test.
    const expandAllBtn = testPage.getByRole("button", { name: "Expand all" });
    await expect(expandAllBtn).toBeVisible({ timeout: 10_000 });
    await expandAllBtn.click();

    // After expanding, all original lines should be visible — pick a line
    // from the middle of the previously collapsed region.
    await expect(testPage.getByText("original_025", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
    await expect(testPage.getByText("original_040", { exact: false })).toBeVisible({
      timeout: 5_000,
    });

    // The "unmodified lines" separators should be gone
    await expect(testPage.getByText(/\d+ unmodified lines/)).toHaveCount(0, {
      timeout: 5_000,
    });
  });
});
