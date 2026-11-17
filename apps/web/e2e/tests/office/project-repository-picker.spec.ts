import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";
import { makeGitEnv } from "../../helpers/git-helper";
import { assertNoDocumentHorizontalOverflow, requireBox } from "../../helpers/layout-assertions";
import {
  addCustomRepository,
  expectRepositoryPickerWithinViewport,
  installRepositoryDiscoveryRoute,
  openCreateProjectDialog,
  openRepositoryPicker,
  pickerCommandList,
  pickerOptions,
  pickerSearchInput,
  repositoryPickerTrigger,
} from "../../helpers/project-repository-picker";

/**
 * Project page — repository picker (chip-style, popover-driven).
 *
 * Surfaces under test:
 *   - `ProjectRepositoryPicker` (popover with cmdk search + custom URL)
 *   - `ProjectReposSection` chip rendering + remove flow
 *
 * Without this, an endpoint rename (`PATCH /office/projects/:id`)
 * or a payload-shape drift would silently break the user's only
 * way to attach a repo to a project.
 *
 * Onboarding does not seed a project (CompleteResult.ProjectID is
 * empty), so each test creates its own via the office API.
 */
async function createProject(
  apiClient: { rawRequest: (m: string, u: string, b?: unknown) => Promise<Response> },
  workspaceId: string,
  name: string,
): Promise<string> {
  const res = await apiClient.rawRequest(
    "POST",
    `/api/v1/office/workspaces/${workspaceId}/projects`,
    { name },
  );
  const body = (await res.json()) as { project?: { id?: string }; id?: string };
  return (body.project?.id ?? body.id) as string;
}

test.describe("Project repository picker", () => {
  test("paste a remote URL → chip appears → remove → empty state returns", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Project");
    await testPage.goto(`/office/projects/${projectId}`);
    await expect(testPage.getByText("Repositories").first()).toBeVisible({ timeout: 10_000 });

    // Empty state until something gets added.
    await expect(testPage.getByText("No repositories added yet.")).toBeVisible();

    // Open the popover and type a URL the picker treats as remote.
    await testPage.getByTestId("project-add-repository").click();
    const url = "https://github.com/example/repo.git";
    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);

    // The "Use this URL" row appears in the Add custom group; the
    // option value matches `__custom__:<query>` so we click by role.
    const customRow = testPage.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible({ timeout: 5_000 });
    await customRow.click();

    // Chip renders with the raw URL. Tooltip carries the full
    // string; the visible label may be truncated.
    const chip = testPage.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(chip).toBeVisible({ timeout: 10_000 });

    // Empty-state line disappears once a chip exists.
    await expect(testPage.getByText("No repositories added yet.")).toHaveCount(0);

    // Re-open the picker: the exact attached value must not offer a
    // duplicate custom row, but a prefix of it must stay addable —
    // partial overlap with existing entries cannot block literal input.
    await testPage.getByTestId("project-add-repository").click();
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);
    await expect(testPage.getByTestId("project-add-custom")).toHaveCount(0);
    await searchInput.fill(url.replace(/\.git$/, ""));
    await expect(testPage.getByTestId("project-add-custom")).toBeVisible();
    await testPage.keyboard.press("Escape");

    // Remove the chip; empty state comes back.
    await chip.getByTestId("project-repo-chip-remove").click();
    await expect(chip).toHaveCount(0, { timeout: 10_000 });
    await expect(testPage.getByText("No repositories added yet.")).toBeVisible();
  });

  test("typing a local path triggers the 'Add as local path' subtitle", async ({
    apiClient,
    testPage,
    officeSeed,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Local");
    await testPage.goto(`/office/projects/${projectId}`);
    await testPage.getByTestId("project-add-repository").click();

    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();

    // A bare absolute path is treated as a local-path entry, not a URL.
    await searchInput.fill("/Users/example/some-project");
    await expect(testPage.getByTestId("project-add-custom")).toBeVisible({ timeout: 5_000 });
    await expect(testPage.getByTestId("project-add-custom")).toContainText(/Add as local path/i);
  });

  test("a near-miss workspace suggestion does not block adding the literal path", async ({
    apiClient,
    testPage,
    officeSeed,
    backend,
  }) => {
    const projectId = await createProject(apiClient, officeSeed.workspaceId, "Repo Picker Overlap");
    // Register a workspace repo whose path is a superstring of the one
    // we'll type — the classic /work/app vs /work/app-old overlap.
    const oldPath = path.join(backend.tmpDir, "repos", "app-old");
    fs.mkdirSync(oldPath, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync("git init -b main", { cwd: oldPath, env: gitEnv });
    execSync('git commit --allow-empty -m "init"', { cwd: oldPath, env: gitEnv });
    await apiClient.createRepository(officeSeed.workspaceId, oldPath, "main", { name: "app-old" });

    await testPage.goto(`/office/projects/${projectId}`);
    await testPage.getByTestId("project-add-repository").click();
    const searchInput = testPage.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();

    const newPath = path.join(backend.tmpDir, "repos", "app");
    await searchInput.fill(newPath);

    // Both the near-miss suggestion and the free-form row must be offered.
    await expect(testPage.getByRole("option", { name: /app-old/ })).toBeVisible();
    const customRow = testPage.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible();
    await customRow.click();

    // The literal path is attached — not the near-miss suggestion.
    const chip = testPage.getByTestId("project-repo-chip");
    await expect(chip).toHaveAttribute("data-repository-value", newPath, { timeout: 10_000 });
  });

  // The create-project dialog embeds the same picker as the detail page.
  // Without this, a dialog-side regression (picker not wired to the form
  // state, repositories dropped from the create payload) would silently
  // break attaching a repo at creation time.
  test("create-project dialog: pick a repo → create → repo persists on detail page", async ({
    testPage,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/projects");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Projects/i, { timeout: 10_000 });
    await testPage
      .getByRole("button", { name: /New Project|Create your first project/ })
      .first()
      .click();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const projectName = `Picker Create ${Date.now()}`;
    await dialog.locator("#project-name").fill(projectName);

    // Add a repository through the picker's custom-entry row.
    const url = "https://github.com/example/created-with-picker.git";
    await dialog.getByTestId("project-add-repository").click();
    const searchInput = dialog.getByPlaceholder(/Search or paste a URL/i);
    await expect(searchInput).toBeVisible();
    await searchInput.fill(url);
    const customRow = dialog.getByTestId("project-add-custom");
    await expect(customRow).toBeVisible({ timeout: 5_000 });
    await customRow.click();

    // Chip renders inside the dialog before submitting.
    const dialogChip = dialog.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(dialogChip).toBeVisible();

    await dialog.getByRole("button", { name: "Create Project" }).click();
    await expect(dialog).toHaveCount(0, { timeout: 10_000 });

    // The new project card appears; opening it shows the persisted repo chip.
    await testPage.getByText(projectName).first().click();
    const detailChip = testPage.locator('[data-testid="project-repo-chip"]', { hasText: url });
    await expect(detailChip).toBeVisible({ timeout: 10_000 });
  });

  test("create-project dialog: long repository chips keep the footer reachable", async ({
    testPage,
    prCapture,
    officeSeed: _,
  }) => {
    await testPage.goto("/office/projects");
    await testPage
      .getByRole("button", { name: /New Project|Create your first project/ })
      .first()
      .click();

    const dialog = testPage.getByTestId("create-project-dialog");
    await expect(dialog).toBeVisible();
    await dialog.locator("#project-name").fill(`Long Picker Create ${Date.now()}`);

    const repositories = Array.from(
      { length: 18 },
      (_, index) => `https://github.com/example/overflow-repository-${index}.git`,
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

    await body.evaluate((element) => {
      const node = element as HTMLElement;
      node.scrollTop = node.scrollHeight;
    });
    await expect
      .poll(() => body.evaluate((element) => (element as HTMLElement).scrollTop))
      .toBeGreaterThan(0);
    await expect(finalChip).toBeInViewport();
    await expect(cancel).toBeInViewport();
    await expect(create).toBeInViewport();
    const footerHitTest = await footer.locator("button").evaluateAll((buttons) =>
      buttons.every((button) => {
        const rect = button.getBoundingClientRect();
        const hit = document.elementFromPoint(
          rect.left + rect.width / 2,
          rect.top + rect.height / 2,
        );
        return hit === button || button.contains(hit);
      }),
    );
    expect(footerHitTest).toBe(true);
    if (prCapture.capturing) {
      await prCapture.screenshot("long-create-project-dialog", {
        caption:
          "The Create Project form scrolls repository chips while its title, final chip, and action footer remain visible.",
      });
    }

    await finalRemove.click();
    await expect(finalChip).toHaveCount(0);
    await cancel.click();
    await expect(dialog).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "create-project dialog");
  });

  test("create-project dialog: repository picker escapes the dialog without clipping", async ({
    testPage,
    prCapture,
    officeSeed: _,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1280, height: 720 });
    const suggestions = await installRepositoryDiscoveryRoute(testPage);
    const dialog = await openCreateProjectDialog(testPage);
    const projectName = `Floating Picker ${Date.now()}`;
    await dialog.locator("#project-name").fill(projectName);

    // One long chip moves the trigger right, so the 420px picker must extend
    // past the dialog's right edge.
    await addCustomRepository(
      dialog,
      "https://github.com/example/very-long-repository-name-for-picker-clipping.git",
    );

    const trigger = repositoryPickerTrigger(dialog);
    const picker = await openRepositoryPicker(trigger);
    const dialogBox = await requireBox(dialog, "create-project dialog");
    const pickerBox = await requireBox(picker, "repository picker");
    expect(pickerBox.width).toBeGreaterThanOrEqual(419);
    expect(pickerBox.width).toBeLessThanOrEqual(421);
    expect(pickerBox.x + pickerBox.width).toBeGreaterThan(dialogBox.x + dialogBox.width + 1);
    await expectRepositoryPickerWithinViewport(picker);
    await assertNoDocumentHorizontalOverflow(testPage, "create-project dialog with open picker");

    const options = pickerOptions(picker);
    const discoveredOptions = picker.locator('[data-value^="/srv/repositories/"]');
    await expect(discoveredOptions).toHaveCount(suggestions.length);
    await expect(options.first()).toBeVisible();
    const hit = await discoveredOptions.first().evaluate((node) => {
      const rect = node.getBoundingClientRect();
      const point = { x: rect.right - 6, y: rect.top + rect.height / 2 };
      const element = document.elementFromPoint(point.x, point.y);
      const resolvedSlot =
        element?.closest("[data-slot]")?.getAttribute("data-slot") ??
        element?.tagName.toLowerCase() ??
        null;
      return {
        point,
        resolved: Boolean(element && (element === node || node.contains(element))),
        resolvedSlot,
      };
    });
    expect(hit.point.x).toBeGreaterThan(dialogBox.x + dialogBox.width);
    expect(
      hit.resolved,
      `right-side point (${hit.point.x.toFixed(1)}, ${hit.point.y.toFixed(1)}) resolved to ${hit.resolvedSlot}`,
    ).toBe(true);

    if (prCapture.capturing) {
      await prCapture.screenshot("floating-create-project-picker", {
        caption:
          "The Create Project repository picker extends beyond the dialog border and remains fully interactive.",
      });
    }

    // Real pointer selection inside the previously clipped region.
    await testPage.mouse.click(hit.point.x, hit.point.y);
    await expect(
      dialog.locator('[data-testid="project-repo-chip"]'),
      "selecting beyond the dialog adds the repository chip",
    ).toHaveCount(2);
    await expect(
      dialog.locator(`[data-repository-value="${suggestions[0].path}"]`),
      "the picked suggestion is the one attached",
    ).toHaveCount(1);
    await expect(picker).toHaveCount(0);
    await expect(dialog.locator("#project-name")).toHaveValue(projectName);
    await expect(dialog.getByTestId("create-project-dialog-footer")).toBeVisible();

    await dialog
      .getByTestId("create-project-dialog-footer")
      .getByRole("button", { name: "Cancel" })
      .click();
    await expect(dialog).toHaveCount(0);
  });

  test("create-project dialog: repository picker scrolling and keyboard dismissal preserve the form", async ({
    testPage,
    officeSeed: _,
  }) => {
    test.setTimeout(120_000);
    await testPage.setViewportSize({ width: 1280, height: 720 });
    const suggestions = await installRepositoryDiscoveryRoute(testPage);
    const dialog = await openCreateProjectDialog(testPage);
    const projectName = `Keyboard Picker ${Date.now()}`;
    await dialog.locator("#project-name").fill(projectName);
    const customRepository = "https://github.com/example/scroll-and-keyboard.git";
    await addCustomRepository(dialog, customRepository);

    const trigger = repositoryPickerTrigger(dialog);
    let picker = await openRepositoryPicker(trigger);
    const searchInput = pickerSearchInput(picker);
    await expect(searchInput).toBeFocused();

    // Wheel input over the portalled surface must reach the command list even
    // though the picker paints outside the dialog shell.
    await expect(picker.locator('[data-value^="/srv/repositories/"]')).toHaveCount(
      suggestions.length,
    );
    const list = pickerCommandList(picker);
    const pickerBox = await requireBox(picker, "repository picker");
    await testPage.mouse.move(
      pickerBox.x + pickerBox.width / 2,
      pickerBox.y + pickerBox.height / 2,
    );
    await testPage.mouse.wheel(0, 240);
    await expect
      .poll(() => list.evaluate((node) => (node as HTMLElement).scrollTop))
      .toBeGreaterThan(0);
    await assertNoDocumentHorizontalOverflow(
      testPage,
      "create-project dialog with scrolled picker",
    );

    await testPage.keyboard.press("Escape");
    await expect(picker).toHaveCount(0);
    await expect(dialog).toBeVisible();
    await expect(trigger).toBeFocused();

    // Keyboard selection from the real opener: search receives focus, typing
    // an exact suggestion path leaves only that option, and Enter selects it
    // instead of submitting the project.
    picker = await openRepositoryPicker(trigger);
    await expect(pickerSearchInput(picker)).toBeFocused();
    const keyboardTarget = suggestions[7];
    await testPage.keyboard.type(keyboardTarget.path);
    const matchedOptions = picker.locator('[data-value^="/srv/repositories/"]');
    await expect(matchedOptions).toHaveCount(1);
    await testPage.keyboard.press("ArrowDown");
    await testPage.keyboard.press("Enter");
    await expect(picker).toHaveCount(0);
    await expect(dialog.locator('[data-testid="project-repo-chip"]')).toHaveCount(2);
    await expect(dialog.locator(`[data-repository-value="${keyboardTarget.path}"]`)).toHaveCount(1);
    await expect(dialog).toBeVisible();
    await expect(dialog.locator("#project-name")).toHaveValue(projectName);
    await expect(
      dialog.getByTestId("create-project-dialog-footer").getByRole("button", {
        name: "Create Project",
      }),
    ).toBeVisible();

    // Outside interaction dismisses only the picker.
    picker = await openRepositoryPicker(trigger);
    await dialog.locator('[data-slot="dialog-title"]').click();
    await expect(picker).toHaveCount(0);
    await expect(dialog).toBeVisible();
    await expect(
      dialog.locator('[data-testid="project-repo-chip"]', { hasText: customRepository }),
    ).toHaveCount(1);

    await dialog
      .getByTestId("create-project-dialog-footer")
      .getByRole("button", { name: "Cancel" })
      .click();
    await expect(dialog).toHaveCount(0);
  });
});
