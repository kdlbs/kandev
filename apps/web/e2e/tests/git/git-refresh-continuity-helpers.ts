import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { createGitEnrichmentGate, type routeGitStatusRefresh } from "./git-status-refresh-helpers";

export type DiffRenderer = "pierre-diffs" | "monaco";

export type VisibleDiffAnchor = {
  line: number;
  side: string;
  content: string;
  offset: number;
};

export type GitRefreshBridge = Awaited<ReturnType<typeof routeGitStatusRefresh>>;

export async function openAllChangesDiff(panel: Locator, page: Page) {
  const direct = panel.getByRole("button", { name: "Diff", exact: true });
  const overflow = panel.getByTestId("panel-header-overflow").first();
  await expect
    .poll(async () => (await direct.isVisible()) || (await overflow.isVisible()), {
      timeout: 15_000,
      message: "the Changes panel should expose its full diff action",
    })
    .toBe(true);
  if (await direct.isVisible()) {
    await direct.click();
    return;
  }
  await overflow.click();
  await page.getByRole("menuitem", { name: "Diff", exact: true }).click();
}

export async function setDiffRenderer(page: Page, renderer: DiffRenderer) {
  await page.addInitScript((provider) => {
    localStorage.setItem(
      "kandev-editor-providers",
      JSON.stringify({
        version: 3,
        state: {
          providers: {
            "code-editor": "monaco",
            "diff-viewer": provider,
            "chat-code-block": "shiki",
            "chat-diff": "pierre-diffs",
            "plan-editor": "tiptap",
          },
        },
      }),
    );
  }, renderer);
}

export async function scrollDiffIntoReadingPosition(
  page: Page,
  renderer: DiffRenderer,
  filePath: string,
  interaction: "programmatic" | "touch" = "programmatic",
) {
  await expect
    .poll(
      async () => {
        await page.locator('[data-testid="review-diff-scroll"]').evaluate((element, path) => {
          const root = element as HTMLElement;
          const section = root.querySelector<HTMLElement>(
            `[data-review-file-key="${encodeURIComponent(path)}"]`,
          );
          if (!section) throw new Error(`Missing diff section for ${path}`);
          root.scrollTop += section.getBoundingClientRect().top - root.getBoundingClientRect().top;
        }, filePath);
        return visibleDiffAnchor(page, renderer, filePath);
      },
      {
        timeout: 30_000,
        message: "the selected diff should render visible line metadata",
      },
    )
    .not.toBeNull();

  if (interaction === "touch") {
    if (renderer === "monaco") {
      await positionMonacoReadingLine(page, filePath, 40);
    } else {
      await positionPierreReadingLine(page, filePath, 40);
    }
    const target =
      renderer === "monaco"
        ? page.locator(
            `[data-review-file-key="${encodeURIComponent(filePath)}"] .monaco-diff-editor`,
          )
        : page.getByTestId("review-diff-scroll");
    await touchSwipe(page, target, 320);
    await touchSwipe(page, target, 320);
    await waitForStableDiffAnchor(page, renderer, filePath);
  } else if (renderer === "monaco") {
    await page.evaluate((path) => {
      const section = document.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      if (!section) throw new Error(`Missing diff section for ${path}`);
      const host = window as Window & {
        monaco?: {
          editor: {
            getEditors: () => Array<{
              getDomNode: () => HTMLElement | null;
              setScrollTop: (value: number) => void;
              getScrollHeight: () => number;
              getLayoutInfo: () => { height: number };
            }>;
          };
        };
      };
      const editors = (host.monaco?.editor.getEditors() ?? []).filter((candidate) =>
        section.contains(candidate.getDomNode()),
      );
      if (editors.length === 0) throw new Error(`Missing Monaco editor for ${path}`);
      for (const editor of editors) {
        editor.setScrollTop(
          Math.min(1200, editor.getScrollHeight() - editor.getLayoutInfo().height),
        );
      }
    }, filePath);
  } else {
    await page.locator('[data-testid="review-diff-scroll"]').evaluate((element, path) => {
      const root = element as HTMLElement;
      const section = root.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      const host = section?.querySelector("diffs-container");
      const targetLine = host?.shadowRoot?.querySelector<HTMLElement>('[data-line="60"]');
      if (targetLine) {
        const rootRect = root.getBoundingClientRect();
        const lineRect = targetLine.getBoundingClientRect();
        root.scrollTop += lineRect.top - rootRect.top - root.clientHeight * 0.35;
      } else {
        root.scrollTop = Math.min(1200, root.scrollHeight - root.clientHeight);
      }
    }, filePath);
  }

  const minimumLine = interaction === "touch" ? 45 : 30;
  await expect
    .poll(async () => (await visibleDiffAnchor(page, renderer, filePath))?.line ?? 0, {
      timeout: 10_000,
      message: "the diff should move to the requested reading position",
    })
    .toBeGreaterThan(minimumLine);
}

async function positionPierreReadingLine(page: Page, filePath: string, lineNumber: number) {
  await page.locator('[data-testid="review-diff-scroll"]').evaluate(
    (element, args) => {
      const root = element as HTMLElement;
      const section = root.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(args.path)}"]`,
      );
      const host = section?.querySelector("diffs-container");
      const line = host?.shadowRoot?.querySelector<HTMLElement>(`[data-line="${args.line}"]`);
      if (!line) throw new Error(`Missing Pierre diff line ${args.line}`);
      const rootRect = root.getBoundingClientRect();
      const lineRect = line.getBoundingClientRect();
      root.scrollTop += lineRect.top - rootRect.top - root.clientHeight * 0.35;
    },
    { path: filePath, line: lineNumber },
  );
}

async function positionMonacoReadingLine(page: Page, filePath: string, lineNumber: number) {
  await page.evaluate(
    ({ path, line }) => {
      const section = document.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      const host = window as Window & {
        monaco?: {
          editor: {
            getEditors: () => Array<{
              getDomNode: () => HTMLElement | null;
              getTopForPosition: (line: number, column: number) => number;
              setScrollTop: (value: number) => void;
            }>;
          };
        };
      };
      const editors = (host.monaco?.editor.getEditors() ?? []).filter((candidate) =>
        section?.contains(candidate.getDomNode()),
      );
      if (editors.length === 0) throw new Error(`Missing Monaco editor for ${path}`);
      for (const editor of editors) {
        editor.setScrollTop(Math.max(0, editor.getTopForPosition(line, 1) - 24));
      }
    },
    { path: filePath, line: lineNumber },
  );
}

async function waitForStableDiffAnchor(page: Page, renderer: DiffRenderer, filePath: string) {
  let candidate: VisibleDiffAnchor | null = null;
  let stableSamples = 0;
  await expect
    .poll(
      async () => {
        const anchor = await visibleDiffAnchor(page, renderer, filePath);
        if (!anchor) {
          candidate = null;
          stableSamples = 0;
          return false;
        }
        if (
          candidate &&
          candidate.line === anchor.line &&
          candidate.side === anchor.side &&
          Math.abs(candidate.offset - anchor.offset) <= 2
        ) {
          stableSamples += 1;
        } else {
          candidate = anchor;
          stableSamples = 1;
        }
        return stableSamples >= 4;
      },
      {
        timeout: 10_000,
        message: "the touch-scrolled diff should settle at a stable reading anchor",
      },
    )
    .toBe(true);
}

async function touchSwipe(page: Page, target: Locator, distance: number) {
  await waitForFiniteAnimations(target);
  const viewport = page.viewportSize();
  if (!viewport) throw new Error("The diff should expose a visible touch scroll region");
  await expect
    .poll(
      async () => {
        const bounds = await target.boundingBox();
        if (!bounds || bounds.height < 80 || bounds.width < 1) return false;
        const visibleTop = Math.max(bounds.y, 8);
        const visibleBottom = Math.min(bounds.y + bounds.height, viewport.height - 8);
        return visibleBottom - visibleTop >= 80;
      },
      { message: "The diff should expose a visible touch scroll region" },
    )
    .toBe(true);
  const bounds = await target.boundingBox();
  if (!bounds) throw new Error("The diff should expose a visible touch scroll region");
  const visibleTop = Math.max(bounds.y, 8);
  const visibleBottom = Math.min(bounds.y + bounds.height, viewport.height - 8);
  const x = bounds.x + bounds.width / 2;
  const startY = visibleBottom - 20;
  const endY = Math.max(visibleTop + 20, startY - distance);
  const session = await page.context().newCDPSession(page);
  try {
    await session.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ x, y: startY, id: 1 }],
    });
    for (let step = 1; step <= 8; step++) {
      await session.send("Input.dispatchTouchEvent", {
        type: "touchMove",
        touchPoints: [{ x, y: startY + ((endY - startY) * step) / 8, id: 1 }],
      });
    }
    await session.send("Input.dispatchTouchEvent", {
      type: "touchEnd",
      touchPoints: [],
    });
  } finally {
    await session.detach();
  }
}

export async function visibleDiffAnchor(
  page: Page,
  renderer: DiffRenderer,
  filePath: string,
): Promise<VisibleDiffAnchor | null> {
  return page.evaluate(
    ({ path, activeRenderer }) => {
      const section = document.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      if (!section) return null;
      const lineSide = (lineType: string): VisibleDiffAnchor["side"] => {
        if (lineType.includes("addition")) return "addition";
        if (lineType.includes("deletion")) return "deletion";
        return "context";
      };
      const readMonacoAnchor = (target: HTMLElement): VisibleDiffAnchor | null => {
        const host = window as Window & {
          monaco?: {
            editor: {
              getEditors: () => Array<{
                getDomNode: () => HTMLElement | null;
                getVisibleRanges: () => Array<{ startLineNumber: number }>;
                getTopForPosition: (line: number, column: number) => number;
                getScrollTop: () => number;
                getModel: () => { getLineContent: (line: number) => string } | null;
              }>;
            };
          };
        };
        const editorEntries = (host.monaco?.editor.getEditors() ?? [])
          .filter((candidate) => target.contains(candidate.getDomNode()))
          .map((candidate) => ({ candidate, visible: candidate.getVisibleRanges()[0] }))
          .filter((entry) => entry.visible)
          .sort((left, right) => right.visible!.startLineNumber - left.visible!.startLineNumber);
        const selected = editorEntries[0];
        const editor = selected?.candidate;
        const visible = selected?.visible;
        if (!editor || !visible) return null;
        return {
          line: visible.startLineNumber,
          side: "modified",
          content: editor.getModel()?.getLineContent(visible.startLineNumber) ?? "",
          offset: editor.getTopForPosition(visible.startLineNumber, 1) - editor.getScrollTop(),
        };
      };

      const readPierreAnchor = (target: HTMLElement): VisibleDiffAnchor | null => {
        const root = document.querySelector<HTMLElement>('[data-testid="review-diff-scroll"]');
        const rootRect = root?.getBoundingClientRect();
        const host = target.querySelector("diffs-container");
        const lines = host?.shadowRoot?.querySelectorAll<HTMLElement>(
          "[data-line][data-line-type]",
        );
        if (!root || !rootRect || !lines) return null;
        const visible = Array.from(lines)
          .map((line) => ({ line, rect: line.getBoundingClientRect() }))
          .filter(({ rect }) => rect.bottom > rootRect.top && rect.top < rootRect.bottom)
          .sort((left, right) => left.rect.top - right.rect.top)[0];
        if (!visible) return null;
        const line = Number(visible.line.dataset.line);
        if (!Number.isFinite(line)) return null;
        const type = visible.line.dataset.lineType ?? "context";
        return {
          line,
          side: lineSide(type),
          content: visible.line.textContent ?? "",
          offset: visible.rect.top - rootRect.top,
        };
      };

      return activeRenderer === "monaco" ? readMonacoAnchor(section) : readPierreAnchor(section);
    },
    { path: filePath, activeRenderer: renderer },
  );
}

export async function rememberDiffViewerNode(page: Page, renderer: DiffRenderer, filePath: string) {
  await page.evaluate(
    ({ path, activeRenderer }) => {
      const section = document.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      const viewer = section?.querySelector(
        activeRenderer === "monaco" ? ".monaco-diff-editor" : "diffs-container",
      );
      if (!viewer) throw new Error(`Missing ${activeRenderer} viewer for ${path}`);
      (window as Window & { __gitRefreshViewerNode?: Element }).__gitRefreshViewerNode = viewer;
    },
    { path: filePath, activeRenderer: renderer },
  );
}

export async function isSameDiffViewerNode(page: Page, renderer: DiffRenderer, filePath: string) {
  return page.evaluate(
    ({ path, activeRenderer }) => {
      const section = document.querySelector<HTMLElement>(
        `[data-review-file-key="${encodeURIComponent(path)}"]`,
      );
      const viewer = section?.querySelector(
        activeRenderer === "monaco" ? ".monaco-diff-editor" : "diffs-container",
      );
      return (
        !!viewer &&
        (window as Window & { __gitRefreshViewerNode?: Element }).__gitRefreshViewerNode === viewer
      );
    },
    { path: filePath, activeRenderer: renderer },
  );
}

export async function triggerForegroundRefresh(page: Page) {
  await page.evaluate(() => window.dispatchEvent(new Event("blur")));
  await page.evaluate(() => window.dispatchEvent(new Event("focus")));
}

export async function holdRefreshDuringEdit(
  page: Page,
  bridge: GitRefreshBridge,
  gate: ReturnType<typeof createGitEnrichmentGate>,
  targetPath: string,
  edit: () => void,
) {
  const priorReadyNotifications = bridge.readyNotificationCount();
  const priorPendingNotifications = bridge.pendingNotificationCount();
  gate.arm();
  edit();
  await triggerForegroundRefresh(page);
  await gate.waitUntilStarted();
  const pendingNotification = await bridge.waitForPendingStatus(priorPendingNotifications);
  const status = pendingNotification.payload?.status;
  const hasPendingTarget =
    status?.detail_state === "pending" &&
    status.files_complete === true &&
    Object.prototype.hasOwnProperty.call(status.files ?? {}, targetPath);
  expect(hasPendingTarget).toBe(true);
  return { priorReadyNotifications };
}

export async function releaseRefreshEnrichment(
  bridge: GitRefreshBridge,
  gate: ReturnType<typeof createGitEnrichmentGate>,
  priorReadyNotifications: number,
) {
  gate.release();
  await expect
    .poll(() => bridge.readyNotificationCount(), {
      timeout: 30_000,
      message: "released Git enrichment should publish a ready status",
    })
    .toBeGreaterThan(priorReadyNotifications);
}
