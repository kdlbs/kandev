/**
 * Mobile-Chrome geometry coverage for the Office Create Project dialog.
 * Repository chips are added through the same picker used by the desktop
 * dialog, so this covers both form growth and the real selection state path.
 */
import { expect, test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import { waitForFiniteAnimations } from "../../helpers/animations";
import {
  addCustomRepository,
  expectRepositoryPickerWithinViewport,
  installRepositoryDiscoveryRoute,
  openCreateProjectDialog,
  openRepositoryPicker,
  pickerCommandList,
  pickerSearchInput,
  repositoryPickerTrigger,
} from "../../helpers/project-repository-picker";

test.describe("Mobile Create Project dialog", () => {
  test("keeps long repository chips and footer controls inside the phone viewport", async ({
    testPage,
    prCapture,
    officeSeed: _,
  }) => {
    test.setTimeout(120_000);

    await openCreateProjectDialog(testPage, "touch");
    const dialog = testPage.getByTestId("create-project-dialog");
    await dialog.locator("#project-name").fill(`Mobile Long Picker ${Date.now()}`);

    const repositories = Array.from(
      { length: 14 },
      (_, index) => `https://github.com/example/mobile-overflow-repository-${index}.git`,
    );
    for (const repository of repositories) {
      await addCustomRepository(dialog, repository, "touch");
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

  test("keeps the repository picker within the phone viewport and persists selection", async ({
    testPage,
    prCapture,
    officeSeed: _,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 393, height: 851 });
    const suggestions = await installRepositoryDiscoveryRoute(testPage);
    const dialog = await openCreateProjectDialog(testPage, "touch");
    const projectName = `Mobile Floating Picker ${Date.now()}`;
    await dialog.locator("#project-name").fill(projectName);

    const trigger = repositoryPickerTrigger(dialog);
    const picker = await openRepositoryPicker(trigger, "touch");
    await expectRepositoryPickerWithinViewport(picker, "phone picker");
    const pickerBox = await requireBox(picker, "phone picker");
    expect(pickerBox.width).toBeLessThanOrEqual(393 - 32 + 1);
    const triggerBox = await requireBox(trigger, "add repository trigger");
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);
    const searchInput = pickerSearchInput(picker);
    await expect(searchInput).toBeVisible();
    const searchBox = await requireBox(searchInput, "picker search input");
    expect(searchBox.height).toBeGreaterThanOrEqual(44);
    await assertNoDocumentHorizontalOverflow(testPage, "phone dialog with open picker");

    // A short phone viewport shrinks the list, not the search control, and
    // still keeps every part of the portalled surface on screen.
    await testPage.setViewportSize({ width: 320, height: 568 });
    await waitForFiniteAnimations(picker);
    await expectRepositoryPickerWithinViewport(picker, "short phone picker");
    const shortPickerBox = await requireBox(picker, "short phone picker");
    expect(shortPickerBox.width).toBeLessThanOrEqual(320 - 32 + 1);
    await expect(searchInput).toBeVisible();
    const list = pickerCommandList(picker);
    await expect
      .poll(() => list.evaluate((node) => node.scrollHeight > node.clientHeight))
      .toBe(true);
    await list.evaluate((node) => {
      node.scrollTop = node.scrollHeight;
    });
    await expect.poll(() => list.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
    await expect(picker.locator('[data-value^="/srv/repositories/"]').last()).toBeInViewport();
    await assertNoDocumentHorizontalOverflow(testPage, "short phone dialog with open picker");

    // Back at the standard phone size, a real tap selects a suggestion. The
    // list keeps its scroll position across the resize, so return it to the
    // first suggestion before tapping.
    await testPage.setViewportSize({ width: 393, height: 851 });
    await waitForFiniteAnimations(picker);
    await expectRepositoryPickerWithinViewport(picker, "phone picker after resize");
    await list.evaluate((node) => {
      node.scrollTop = 0;
    });
    await expect.poll(() => list.evaluate((node) => node.scrollTop)).toBe(0);
    const firstOption = picker.locator('[data-value^="/srv/repositories/"]').first();
    await expect(firstOption).toBeInViewport();
    if (prCapture.capturing) {
      await prCapture.screenshot("phone-floating-create-project-picker", {
        caption:
          "The phone repository picker fits the viewport with its search control and scrolling option list.",
      });
    }
    await firstOption.tap();
    await expect(picker).toHaveCount(0);
    await expect(dialog.locator(`[data-repository-value="${suggestions[0].path}"]`)).toHaveCount(1);
    await expect(dialog.locator("#project-name")).toHaveValue(projectName);

    await dialog
      .getByTestId("create-project-dialog-footer")
      .getByRole("button", { name: "Create Project" })
      .tap();
    await expect(dialog).toHaveCount(0, { timeout: 10_000 });

    const detailChip = testPage.locator(`[data-repository-value="${suggestions[0].path}"]`);
    const projectLink = testPage.getByRole("link", { name: new RegExp(projectName) });
    await expect(projectLink).toBeVisible({ timeout: 10_000 });
    await projectLink.tap();
    await expect(detailChip).toBeVisible({ timeout: 10_000 });
    await testPage.reload();
    await expect(
      testPage.locator(`[data-repository-value="${suggestions[0].path}"]`),
      "the selected repository persists after reload",
    ).toBeVisible({ timeout: 10_000 });
  });
});
