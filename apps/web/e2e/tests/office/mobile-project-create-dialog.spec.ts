/**
 * Mobile-Chrome geometry coverage for the Office Create Project dialog.
 * Repository chips are added through the same picker used by the desktop
 * dialog, so this covers both form growth and the real selection state path.
 */
import type { Locator } from "@playwright/test";
import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

async function addCustomRepository(dialog: Locator, value: string): Promise<void> {
  await dialog.getByTestId("project-add-repository").tap();
  const searchInput = dialog.getByPlaceholder(/Search or paste a URL/i);
  await expect(searchInput).toBeVisible();
  await searchInput.fill(value);
  const customRow = dialog.getByTestId("project-add-custom");
  await expect(customRow).toBeVisible({ timeout: 5_000 });
  await customRow.tap();
}

test.describe("Mobile Create Project dialog", () => {
  test("keeps long repository chips and footer controls inside the phone viewport", async ({
    testPage,
    prCapture,
    officeSeed: _,
  }) => {
    test.setTimeout(120_000);

    await testPage.goto("/office/projects");
    await testPage
      .getByRole("button", { name: /New Project|Create your first project/ })
      .first()
      .tap();

    const dialog = testPage.getByTestId("create-project-dialog");
    await expect(dialog).toBeVisible();
    await dialog.locator("#project-name").fill(`Mobile Long Picker ${Date.now()}`);

    const repositories = Array.from(
      { length: 14 },
      (_, index) => `https://github.com/example/mobile-overflow-repository-${index}.git`,
    );
    for (const repository of repositories) {
      await addCustomRepository(dialog, repository);
    }

    const body = dialog.getByTestId("create-project-dialog-body");
    const footer = dialog.getByTestId("create-project-dialog-footer");
    const title = dialog.locator('[data-slot="dialog-title"]');
    const finalChip = body.locator('[data-testid="project-repo-chip"]', {
      hasText: repositories.at(-1) ?? "",
    });
    const finalRemove = finalChip.getByTestId("project-repo-chip-remove");
    const cancel = footer.getByRole("button", { name: "Cancel" });
    const create = footer.getByRole("button", { name: "Create Project" });
    await expect(body).toHaveCount(1);
    await expect(footer).toHaveCount(1);
    await expect(finalChip).toBeVisible();

    const bodyMetrics = await body.evaluate((element) => {
      const node = element as HTMLElement;
      return { clientHeight: node.clientHeight, scrollHeight: node.scrollHeight };
    });
    expect(bodyMetrics.scrollHeight).toBeGreaterThan(bodyMetrics.clientHeight);

    const footerOutsideBody = await dialog.evaluate((element) => {
      const bodyNode = element.querySelector('[data-testid="create-project-dialog-body"]');
      const footerNode = element.querySelector('[data-testid="create-project-dialog-footer"]');
      return Boolean(bodyNode && footerNode && !bodyNode.contains(footerNode));
    });
    expect(footerOutsideBody).toBe(true);

    const viewport = await testPage.evaluate(() => ({ width: innerWidth, height: innerHeight }));
    for (const [label, locator] of [
      ["dialog", dialog],
      ["title", title],
      ["footer", footer],
    ] as const) {
      const box = await locator.boundingBox();
      if (!box) throw new Error(`${label} has no layout box`);
      expect(box.x, `${label} left edge`).toBeGreaterThanOrEqual(0);
      expect(box.y, `${label} top edge`).toBeGreaterThanOrEqual(0);
      expect(box.x + box.width, `${label} right edge`).toBeLessThanOrEqual(viewport.width);
      expect(box.y + box.height, `${label} bottom edge`).toBeLessThanOrEqual(viewport.height);
    }

    for (const [label, locator] of [
      ["Cancel", cancel],
      ["Create Project", create],
    ] as const) {
      const box = await locator.boundingBox();
      if (!box) throw new Error(`${label} has no layout box`);
      expect(box.height).toBeGreaterThanOrEqual(44);
    }

    await body.evaluate((element) => {
      const node = element as HTMLElement;
      node.scrollTop = node.scrollHeight;
    });
    await expect
      .poll(() => body.evaluate((element) => (element as HTMLElement).scrollTop))
      .toBeGreaterThan(0);
    await expect(finalChip).toBeInViewport();
    await expect(finalRemove).toBeInViewport();
    await expect(cancel).toBeInViewport();
    await expect(create).toBeInViewport();
    if (prCapture.capturing) {
      await prCapture.screenshot("long-create-project-dialog", {
        caption:
          "The mobile Create Project form keeps the repository body and action footer usable.",
      });
    }

    await finalRemove.tap();
    await expect(finalChip).toHaveCount(0);
    await cancel.tap();
    await expect(dialog).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile create-project dialog");
  });
});
