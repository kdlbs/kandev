import { expect, type Page } from "@playwright/test";
import type { CommitSpacingSource } from "./commit-spacing-source";
import type { AppState } from "../../../lib/state/store";
import { expectBoundedTimeline, seedLargeWorkingTree } from "./large-changes-helpers";

export async function seedCommitSpacingHistory(
  page: Page,
  source: CommitSpacingSource,
): Promise<void> {
  const fixture = await page.evaluate(() => {
    const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => AppState } })
      .__KANDEV_E2E_STORE__;
    if (!store) throw new Error("E2E store bridge missing");
    const state = store.getState();
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId || !state.environmentIdBySessionId[sessionId]) {
      throw new Error("Active environment is unavailable");
    }
    return {
      sessionId,
      commits: Array.from({ length: 160 }, (_, index) => ({
        id: `spacing-${index}`,
        session_id: sessionId,
        commit_sha: `${index.toString(16).padStart(7, "0")}${"0".repeat(33)}`,
        parent_sha: "0".repeat(40),
        author_name: "Spacing fixture",
        author_email: "spacing@example.invalid",
        commit_message: `Change ${String(index).padStart(3, "0")}: keep history rows contiguous`,
        committed_at: "2026-01-01T00:00:00Z",
        created_at: "2026-01-01T00:00:00Z",
        files_changed: 1,
        insertions: index + 1,
        deletions: index % 3,
        pushed: true,
      })),
    };
  });
  source.use(fixture.sessionId, fixture.commits);
  const priorResponses = source.responseCount();
  await seedLargeWorkingTree(page, "flat", 0);
  await page.evaluate(({ sessionId, commits }) => {
    const state = (
      window as Window & { __KANDEV_E2E_STORE__: { getState: () => AppState } }
    ).__KANDEV_E2E_STORE__.getState();
    state.setSessionCommits(sessionId, commits);
    // Exercise the real read path so a late backend refresh cannot replace the fixture.
    state.bumpSessionCommitsRefetch(sessionId);
  }, fixture);
  await expect.poll(() => source.responseCount()).toBeGreaterThan(priorResponses);
  await expect
    .poll(() =>
      page.evaluate((sessionId) => {
        const state = (
          window as Window & { __KANDEV_E2E_STORE__: { getState: () => AppState } }
        ).__KANDEV_E2E_STORE__.getState();
        const env = state.environmentIdBySessionId[sessionId] ?? sessionId;
        return {
          count: state.sessionCommits.byEnvironmentId[env]?.length,
          loading: state.sessionCommits.loading[env] === true,
        };
      }, fixture.sessionId),
    )
    .toEqual({ count: 160, loading: false });
  const toggle = page.getByTestId("commits-section-collapse-toggle");
  await expect(toggle).toBeVisible();
  if ((await toggle.getAttribute("aria-expanded")) === "false") await toggle.click();
  await expect(page.getByTestId("commit-row-0000000")).toBeVisible();
}

export async function visibleTimelineGeometry(page: Page) {
  return page.getByTestId("changes-panel-scroll-owner").evaluate((owner) => {
    const ownerBox = owner.getBoundingClientRect();
    const top = ownerBox.top + owner.clientTop;
    const bottom = top + owner.clientHeight;
    const rows = [...owner.querySelectorAll<HTMLElement>("[data-changes-timeline-row]")]
      .map((element) => {
        const bounds = element.getBoundingClientRect();
        return {
          key: element.dataset.changesRowKey,
          index: Number(element.dataset.index),
          top: bounds.top,
          bottom: bounds.bottom,
        };
      })
      .filter((row) => row.bottom > top && row.top < bottom)
      .sort((left, right) => left.index - right.index);
    const gaps = rows.slice(1).map((row, index) => row.top - rows[index].bottom);
    return {
      count: rows.length,
      maxGap: Math.max(0, ...gaps.map(Math.abs)),
      coversTop: rows.length > 0 && rows[0].top <= top + 1,
      indicesContiguous: rows.every(
        (row, index) => index === 0 || row.index === rows[index - 1].index + 1,
      ),
      anchor: rows[0] ? { key: rows[0].key, offset: rows[0].top - top } : null,
    };
  });
}

export async function expectContiguousTimeline(page: Page): Promise<void> {
  await expectBoundedTimeline(page);
  await expect
    .poll(async () => {
      const geometry = await visibleTimelineGeometry(page);
      return {
        enoughRows: geometry.count > 3,
        coversTop: geometry.coversTop,
        indicesContiguous: geometry.indicesContiguous,
        touching: geometry.maxGap <= 1,
      };
    })
    .toEqual({ enoughRows: true, coversTop: true, indicesContiguous: true, touching: true });
}

export async function refreshSpacingAndExpectAnchor(page: Page): Promise<void> {
  await expectContiguousTimeline(page);
  const before = (await visibleTimelineGeometry(page)).anchor;
  expect(before).not.toBeNull();
  await page.evaluate(async () => {
    document.fonts.dispatchEvent(new Event("loadingdone"));
    await new Promise<void>((resolve) =>
      requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
    );
  });
  await expectContiguousTimeline(page);
  await expect
    .poll(async () => {
      const after = (await visibleTimelineGeometry(page)).anchor;
      return (
        after?.key === before?.key && Math.abs((after?.offset ?? Infinity) - before!.offset) <= 1
      );
    })
    .toBe(true);
}
