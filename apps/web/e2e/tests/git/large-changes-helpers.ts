import { expect, type Locator, type Page } from "@playwright/test";

export const LARGE_CHANGES_FILE_COUNT = 50_000;

export function largeChangesFilePath(index: number): string {
  const name =
    index === LARGE_CHANGES_FILE_COUNT - 1
      ? "large-change-49999-with-a-long-mobile-friendly-filename.ts"
      : `large-change-${String(index).padStart(5, "0")}.ts`;
  return `generated/bucket-${String(index % 1_000).padStart(4, "0")}/${name}`;
}

export function largeChangesFileTestId(index: number): string {
  return `file-row-${largeChangesFilePath(index).replaceAll("/", "-")}`;
}

export async function seedLargeWorkingTree(
  page: Page,
  layout: "flat" | "tree",
  count = LARGE_CHANGES_FILE_COUNT,
): Promise<void> {
  await page.evaluate(
    ({ fileCount, selectedLayout }) => {
      type E2EState = {
        tasks: { activeSessionId: string | null };
        environmentIdBySessionId: Record<string, string>;
        userSettings: { changesPanelLayout: "flat" | "tree"; [key: string]: unknown };
        setUserSettings: (settings: E2EState["userSettings"]) => void;
        setSessionCommits: (
          sessionId: string,
          commits: unknown[],
          options?: { allowEmpty?: boolean },
        ) => void;
        setGitStatus: (
          environmentId: string,
          status: {
            branch: string;
            remote_branch: string | null;
            modified: string[];
            added: string[];
            deleted: string[];
            untracked: string[];
            renamed: string[];
            ahead: number;
            behind: number;
            files: Record<
              string,
              {
                path: string;
                status: "untracked";
                staged: boolean;
                additions: number;
                deletions: number;
              }
            >;
            timestamp: string;
          },
        ) => boolean;
      };
      const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => E2EState } })
        .__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const state = store.getState();
      const sessionId = state.tasks.activeSessionId;
      if (!sessionId) throw new Error("Active task session is unavailable");
      const environmentId = state.environmentIdBySessionId[sessionId] ?? sessionId;
      state.setUserSettings({ ...state.userSettings, changesPanelLayout: selectedLayout });
      state.setSessionCommits(sessionId, [], { allowEmpty: true });

      const untracked = new Array<string>(fileCount);
      const files: Record<
        string,
        {
          path: string;
          status: "untracked";
          staged: boolean;
          additions: number;
          deletions: number;
        }
      > = Object.create(null) as Record<
        string,
        {
          path: string;
          status: "untracked";
          staged: boolean;
          additions: number;
          deletions: number;
        }
      >;
      for (let index = 0; index < fileCount; index += 1) {
        const name =
          index === fileCount - 1
            ? "large-change-49999-with-a-long-mobile-friendly-filename.ts"
            : `large-change-${String(index).padStart(5, "0")}.ts`;
        const path = `generated/bucket-${String(index % 1_000).padStart(4, "0")}/${name}`;
        untracked[index] = path;
        files[path] = { path, status: "untracked", staged: false, additions: 1, deletions: 0 };
      }
      state.setGitStatus(environmentId, {
        branch: "main",
        remote_branch: null,
        modified: [],
        added: [],
        deleted: [],
        untracked,
        renamed: [],
        ahead: 0,
        behind: 0,
        files,
        timestamp: new Date(Date.now() + 3_600_000).toISOString(),
      });
    },
    { fileCount: count, selectedLayout: layout },
  );
}

export function timelineRows(page: Page): Locator {
  return page.locator('[data-testid="changes-panel-scroll-owner"] [data-changes-timeline-row]');
}

export async function expectBoundedTimeline(page: Page): Promise<void> {
  await expect.poll(() => timelineRows(page).count()).toBeLessThanOrEqual(120);
}

export async function scrollChangesToStart(page: Page, firstFileTestId: string): Promise<void> {
  const owner = page.getByTestId("changes-panel-scroll-owner");
  await owner.evaluate((element) => {
    element.scrollTop = 0;
    element.dispatchEvent(new Event("scroll"));
  });
  const firstFile = page.getByTestId(firstFileTestId);
  await expect(firstFile).toBeVisible({ timeout: 15_000 });
  await expectBoundedTimeline(page);
  await expect
    .poll(async () => {
      const [ownerBox, fileBox] = await Promise.all([owner.boundingBox(), firstFile.boundingBox()]);
      if (!ownerBox || !fileBox) return false;
      return (
        fileBox.y >= ownerBox.y - 1 &&
        fileBox.y + fileBox.height <= ownerBox.y + ownerBox.height + 1
      );
    })
    .toBe(true);
}

export async function scrollChangesToEnd(page: Page, lastFileTestId: string): Promise<void> {
  const owner = page.getByTestId("changes-panel-scroll-owner");
  await expect(owner).toBeVisible();
  await owner.evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.dispatchEvent(new Event("scroll"));
  });
  const lastFile = page.getByTestId(lastFileTestId);
  await expect(lastFile).toBeVisible({ timeout: 15_000 });
  await lastFile.evaluate(async (row) => {
    const owner = row.closest<HTMLElement>('[data-testid="changes-panel-scroll-owner"]');
    if (!owner) throw new Error("Changes scroll owner missing from the final row");
    for (let attempt = 0; attempt < 4; attempt += 1) {
      const rowBox = row.getBoundingClientRect();
      const ownerBox = owner.getBoundingClientRect();
      let correction = 0;
      if (rowBox.bottom > ownerBox.bottom) {
        correction = rowBox.bottom - ownerBox.bottom + 1;
      } else if (rowBox.top < ownerBox.top) {
        correction = rowBox.top - ownerBox.top - 1;
      }
      if (correction === 0) return;
      owner.scrollTop += correction;
      owner.dispatchEvent(new Event("scroll"));
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    }
  });
  await expectBoundedTimeline(page);
  const getGeometry = async () => {
    const [ownerBox, fileBox] = await Promise.all([owner.boundingBox(), lastFile.boundingBox()]);
    const scroll = await owner.evaluate((element) => ({
      top: element.scrollTop,
      height: element.scrollHeight,
      clientHeight: element.clientHeight,
      windowWidth: window.innerWidth,
    }));
    if (!ownerBox || !fileBox) return { within: false, ownerBox, fileBox, scroll };
    return {
      within:
        fileBox.y >= ownerBox.y - 1 &&
        fileBox.y + fileBox.height <= ownerBox.y + ownerBox.height + 1,
      ownerBox,
      fileBox,
      scroll,
    };
  };
  try {
    await expect.poll(getGeometry).toMatchObject({ within: true });
  } catch {
    const geometry = await getGeometry();
    expect(geometry.within, JSON.stringify(geometry)).toBe(true);
  }
}

export async function expectNoPageHorizontalOverflow(page: Page): Promise<void> {
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  );
}

export type BrowserObservation = {
  label: string;
  mountedRows: number;
  domNodes: number;
  heapUsedBytes: number | null;
  longTaskCount: number | null;
  longestTaskMs: number | null;
};

export async function installLongTaskObserver(page: Page): Promise<void> {
  await page.evaluate(() => {
    const target = window as Window & {
      __largeChangesLongTasks?: number[];
    };
    target.__largeChangesLongTasks = [];
    if (!PerformanceObserver.supportedEntryTypes.includes("longtask")) return;
    const observer = new PerformanceObserver((list) => {
      const entries = (window as Window & { __largeChangesLongTasks: number[] })
        .__largeChangesLongTasks;
      entries.push(...list.getEntries().map((entry) => entry.duration));
    });
    observer.observe({ type: "longtask", buffered: true });
  });
}

export async function captureBrowserObservation(
  page: Page,
  label: string,
): Promise<BrowserObservation> {
  const mountedRows = await timelineRows(page).count();
  return page.evaluate(
    ({ observationLabel, rowCount }) => {
      const memory = (performance as Performance & { memory?: { usedJSHeapSize: number } }).memory;
      const longTasks = (window as Window & { __largeChangesLongTasks?: number[] })
        .__largeChangesLongTasks;
      return {
        label: observationLabel,
        mountedRows: rowCount,
        domNodes: document.getElementsByTagName("*").length,
        heapUsedBytes: memory?.usedJSHeapSize ?? null,
        longTaskCount: longTasks?.length ?? null,
        longestTaskMs:
          longTasks?.reduce((maximum, duration) => Math.max(maximum, duration), 0) ?? null,
      };
    },
    { observationLabel: label, rowCount: mountedRows },
  );
}
