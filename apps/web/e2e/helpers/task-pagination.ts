import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { waitForHttp } from "./causal-waits";

export async function seedPaginationTasks(api: ApiClient, seed: SeedData) {
  for (let index = 0; index < 70; index += 1) {
    await api.createTask(seed.workspaceId, `Pagination task ${index + 1}`, {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
  }
}

export async function scrollToPagination(page: Page) {
  await page.getByTestId("tasks-list-view").evaluate((element) => {
    element.scrollTop = element.scrollHeight;
    element.scrollLeft = 0;
  });
  await expect(page.getByTestId("tasks-pagination")).toBeVisible();
}

function waitForPage(page: Page, number: number, size = 10) {
  return waitForHttp(page, "GET", /\/api\/v1\/workspaces\/[^/]+\/tasks$/, {
    predicate: (response) => {
      const params = new URL(response.url()).searchParams;
      return params.get("page") === String(number) && params.get("page_size") === String(size);
    },
  });
}

export async function preparePagination(page: Page, workspaceId: string, touch: boolean) {
  await page.goto(`/tasks?workspace=${workspaceId}&workflow=all&repository=all&group=none`);
  await scrollToPagination(page);
  const size = page.getByTestId("tasks-pagination-page-size");
  if (touch) await size.tap();
  else await size.click();
  const loaded = waitForPage(page, 1);
  const option = page.getByRole("option", { name: "10", exact: true });
  if (touch) await option.tap();
  else await option.click();
  await loaded;
  await expectPage(page, 1);
}

export async function expectPage(page: Page, number: number) {
  const footer = page.getByTestId("tasks-pagination");
  await expect(footer.locator('[aria-current="page"]')).toHaveText(String(number));
  await expect(footer).toContainText(
    `Showing ${(number - 1) * 10 + 1} to ${number * 10} of 70 results`,
  );
  await scrollToPagination(page);
}

export async function navigate(page: Page, control: Locator, number: number, touch: boolean) {
  const loaded = waitForPage(page, number);
  if (touch) await control.tap();
  else await control.click();
  await loaded;
  await expectPage(page, number);
}

export async function expectContained(page: Page, minimum: number) {
  const footer = page.getByTestId("tasks-pagination");
  const list = page.getByTestId("tasks-list-view");
  const viewport = page.viewportSize()!;
  const surface = (await list.boundingBox())!;
  const footerBox = (await footer.boundingBox())!;
  expect(footerBox.x).toBeGreaterThanOrEqual(surface.x - 1);
  expect(footerBox.x + footerBox.width).toBeLessThanOrEqual(surface.x + surface.width + 1);
  expect(footerBox.x).toBeGreaterThanOrEqual(0);
  expect(footerBox.x + footerBox.width).toBeLessThanOrEqual(viewport.width);
  const controls = footer.locator("button:visible");
  const boxes = [];
  for (const control of await controls.all()) {
    const box = (await control.boundingBox())!;
    expect(box.width).toBeGreaterThanOrEqual(minimum);
    expect(box.height).toBeGreaterThanOrEqual(minimum);
    expect(box.x).toBeGreaterThanOrEqual(Math.max(0, surface.x) - 1);
    expect(box.x + box.width).toBeLessThanOrEqual(
      Math.min(viewport.width, surface.x + surface.width) + 1,
    );
    expect(box.y).toBeGreaterThanOrEqual(surface.y - 1);
    expect(box.y + box.height).toBeLessThanOrEqual(
      Math.min(viewport.height, surface.y + surface.height) + 1,
    );
    if (await control.isEnabled()) {
      expect(
        await control.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          const hit = document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2);
          return hit !== null && element.contains(hit);
        }),
      ).toBe(true);
    }
    boxes.push(box);
  }
  for (let i = 0; i < boxes.length; i += 1) {
    for (const other of boxes.slice(i + 1)) {
      const box = boxes[i];
      expect(
        box.x + box.width <= other.x ||
          other.x + other.width <= box.x ||
          box.y + box.height <= other.y ||
          other.y + other.height <= box.y,
      ).toBe(true);
    }
  }
  expect(
    await list.evaluate((element) => ({
      width: element.scrollWidth,
      client: element.clientWidth,
      left: element.scrollLeft,
    })),
  ).toEqual({
    width: await list.evaluate((element) => element.clientWidth),
    client: await list.evaluate((element) => element.clientWidth),
    left: 0,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
