import { expect, test } from "../../fixtures/test-base";
import { useRegularMode } from "../../helpers/regular-mode";
import type { Locator, Page } from "@playwright/test";

type TitlebarRect = { x: number; y: number; width: number; height: number };

useRegularMode();

async function installWindowControlsOverlay(page: Page, initialRect: TitlebarRect): Promise<void> {
  await page.addInitScript((rect) => {
    class FakeWindowControlsOverlay extends EventTarget {
      visible = true;
      titlebarRect = rect;

      getTitlebarAreaRect() {
        return this.titlebarRect;
      }
    }

    const overlay = new FakeWindowControlsOverlay();
    Object.defineProperty(navigator, "windowControlsOverlay", {
      configurable: true,
      value: overlay,
    });
    (
      window as Window & {
        __KANDEV_E2E_WINDOW_CONTROLS_OVERLAY__?: {
          publish: (next: TitlebarRect, visible?: boolean) => void;
        };
      }
    ).__KANDEV_E2E_WINDOW_CONTROLS_OVERLAY__ = {
      publish(next, visible = true) {
        overlay.titlebarRect = next;
        overlay.visible = visible;
        overlay.dispatchEvent(new Event("geometrychange"));
      },
    };
  }, initialRect);
}

async function readInteractiveBounds(
  header: Locator,
): Promise<{ minLeft: number; maxRight: number }> {
  return header.evaluate((element) => {
    const interactive = Array.from(
      element.querySelectorAll<HTMLElement>(
        "a,button,input,select,textarea,[role='button'],[tabindex]:not([tabindex='-1'])",
      ),
    )
      .filter((candidate) => {
        const rect = candidate.getBoundingClientRect();
        return (
          rect.width > 0 && rect.height > 0 && getComputedStyle(candidate).visibility !== "hidden"
        );
      })
      .map((candidate) => candidate.getBoundingClientRect());
    if (interactive.length === 0) throw new Error("titlebar has no visible interactive controls");
    return {
      minLeft: Math.min(...interactive.map((rect) => rect.left)),
      maxRight: Math.max(...interactive.map((rect) => rect.right)),
    };
  });
}

test.describe("desktop PWA window controls overlay", () => {
  test("fuses desktop chrome around live left and right window-control geometry", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await testPage.emulateMedia({ colorScheme: "light" });
    await testPage.addInitScript(() => window.localStorage.setItem("theme", "dark"));
    await installWindowControlsOverlay(testPage, { x: 72, y: 0, width: 1528, height: 40 });
    await testPage.goto("/");

    const shell = testPage.getByTestId("app-shell");
    const sidebarHeader = testPage.locator('[data-window-controls-overlay-region="sidebar"]');
    const pageHeader = testPage.locator('[data-window-controls-overlay-region="content"]').first();
    await expect(shell).toHaveAttribute("data-window-controls-overlay", "visible");
    await expect(
      testPage.locator('meta[data-kandev-window-controls-theme-color="true"]'),
    ).toHaveCount(1);
    await expect(
      testPage.locator('meta[data-kandev-window-controls-theme-color="true"]'),
    ).toHaveAttribute("content", "#181818");
    await expect(sidebarHeader).toBeVisible();
    await expect(pageHeader).toBeVisible();

    expect((await readInteractiveBounds(sidebarHeader)).minLeft).toBeGreaterThanOrEqual(72);
    await expect
      .poll(() =>
        pageHeader.evaluate((element) =>
          getComputedStyle(element).getPropertyValue("-webkit-app-region"),
        ),
      )
      .toBe("drag");
    await expect
      .poll(() =>
        sidebarHeader
          .locator("button")
          .first()
          .evaluate((element) => getComputedStyle(element).getPropertyValue("-webkit-app-region")),
      )
      .toBe("no-drag");

    await testPage.getByRole("button", { name: "Collapse sidebar" }).click();
    await expect(testPage.getByTestId("app-sidebar")).toHaveAttribute("data-collapsed", "true");
    expect((await readInteractiveBounds(sidebarHeader)).minLeft).toBeGreaterThanOrEqual(72);

    await testPage.evaluate(() => {
      (
        window as Window & {
          __KANDEV_E2E_WINDOW_CONTROLS_OVERLAY__?: {
            publish: (next: TitlebarRect, visible?: boolean) => void;
          };
        }
      ).__KANDEV_E2E_WINDOW_CONTROLS_OVERLAY__?.publish({
        x: 0,
        y: 0,
        width: 1510,
        height: 40,
      });
    });
    await expect
      .poll(async () => (await readInteractiveBounds(pageHeader)).maxRight)
      .toBeLessThanOrEqual(1510);
    expect(await testPage.evaluate(() => document.documentElement.scrollWidth)).toBe(
      await testPage.evaluate(() => document.documentElement.clientWidth),
    );
  });

  test("keeps task topbar controls before the right-side system controls", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await installWindowControlsOverlay(testPage, { x: 0, y: 0, width: 1510, height: 40 });
    const task = await apiClient.createTask(seedData.workspaceId, "Overlay Task Topbar", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
    });
    await testPage.goto(`/t/${task.id}`);

    const taskHeader = testPage.getByTestId("task-topbar");
    await expect(taskHeader).toBeVisible();
    await expect(taskHeader).toHaveAttribute("data-window-controls-overlay-region", "content");
    await expect
      .poll(() =>
        taskHeader.evaluate((element) =>
          getComputedStyle(element).getPropertyValue("-webkit-app-region"),
        ),
      )
      .toBe("drag");
    expect((await readInteractiveBounds(taskHeader)).maxRight).toBeLessThanOrEqual(1510);
  });

  test("preserves the existing desktop topbar geometry without the overlay API", async ({
    testPage,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await testPage.goto("/");

    await expect(testPage.getByTestId("app-shell")).toHaveAttribute(
      "data-window-controls-overlay",
      "hidden",
    );
    expect((await testPage.getByTestId("app-sidebar-header").boundingBox())?.height).toBe(40);
  });
});
