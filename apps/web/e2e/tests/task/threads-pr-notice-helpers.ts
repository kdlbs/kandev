import { expect, type Locator } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

/** Resize the tile through its host while retaining the desktop viewport and real flex layout. */
export async function setDesktopTileWidth(
  board: Locator,
  tile: Locator,
  width: number,
  columns: number,
) {
  // Constrain the host, leaving real tile and composer allocation intact.
  await board.evaluate(
    (element, hostWidth) => {
      element.style.maxWidth = `${hostWidth}px`;
    },
    width * columns + 24 + (columns - 1) * 12,
  );
  await expect.poll(async () => (await tile.boundingBox())!.width).toBeCloseTo(width, 0);
  expect(tile.page().viewportSize()!.width).toBeGreaterThan(768);
}

/** Attach one merged or closed PR to a task through the mock GitHub provider. */
export async function associateTerminalPR(
  apiClient: ApiClient,
  taskId: string,
  state: "merged" | "closed",
) {
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    owner: "test-org",
    repo: "test-repo",
    pr_number: 4702,
    pr_url: "https://github.com/test-org/test-repo/pull/4702",
    pr_title: "Readable terminal notice",
    head_branch: "feature/notice",
    base_branch: "main",
    author_login: "test-user",
    state,
  });
}

/** Require room for ordinary words and contain the message and controls within the banner. */
export async function expectReadableNotice(banner: Locator) {
  await expect(banner).toBeVisible();
  const message = banner.locator(":scope > span").first();
  const metrics = await message.evaluate((element) => {
    const context = document.createElement("canvas").getContext("2d")!;
    context.font = getComputedStyle(element).font;
    const words = element.textContent!.trim().split(/\s+/);
    return {
      width: element.getBoundingClientRect().width,
      longestWord: Math.max(...words.map((word) => context.measureText(word).width)),
    };
  });
  expect(metrics.width, "notice text must fit ordinary words").toBeGreaterThanOrEqual(
    metrics.longestWord,
  );
  const bounds = (await banner.boundingBox())!;
  for (const child of [message, ...(await banner.locator(":scope > button").all())]) {
    const box = (await child.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(bounds.x);
    expect(box.x + box.width).toBeLessThanOrEqual(bounds.x + bounds.width);
    expect(box.y + box.height).toBeLessThanOrEqual(bounds.y + bounds.height);
  }
}

/** Require the minimum hit size and an unobscured center for each banner button. */
export async function expectNoticeTargets(banner: Locator, minimum: number) {
  for (const button of await banner.locator(":scope > button").all()) {
    const box = (await button.boundingBox())!;
    expect(box.width).toBeGreaterThanOrEqual(minimum);
    expect(box.height).toBeGreaterThanOrEqual(minimum);
    expect(
      await button.evaluate((element) => {
        const box = element.getBoundingClientRect();
        return element.contains(
          document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2),
        );
      }),
    ).toBe(true);
  }
}
