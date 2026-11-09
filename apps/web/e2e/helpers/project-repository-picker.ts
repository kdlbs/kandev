import { expect, type Locator, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "./animations";

/**
 * Shared setup for the Office project repository picker. The picker opens a
 * cmdk popover inside the Create Project dialog or on the project detail page,
 * so its suggestions must be deterministic and its geometry must be measured
 * from the real portalled surface.
 */

export const DISCOVERED_REPOSITORY_COUNT = 18;

export type DiscoveredRepositorySuggestion = {
  path: string;
  name: string;
  default_branch: string;
};

/** Absolute paths keep the picker on its discovered "on disk" group. */
export function discoveredRepositorySuggestions(
  count = DISCOVERED_REPOSITORY_COUNT,
): DiscoveredRepositorySuggestion[] {
  return Array.from({ length: count }, (_, index) => {
    const suffix = String(index).padStart(2, "0");
    return {
      path: `/srv/repositories/discovered-project-${suffix}`,
      name: `discovered-project-${suffix}`,
      default_branch: "main",
    };
  });
}

/**
 * Seeds the lazy discovery read the picker issues when it opens. The route must
 * be installed before the first navigation that mounts the picker.
 */
export async function installRepositoryDiscoveryRoute(
  page: Page,
  count = DISCOVERED_REPOSITORY_COUNT,
): Promise<DiscoveredRepositorySuggestion[]> {
  const repositories = discoveredRepositorySuggestions(count);
  const body = JSON.stringify({
    roots: ["/srv/repositories"],
    repositories,
    total: repositories.length,
    desktop_runtime: false,
    root_states: [],
    scan_time: "2026-10-28T09:00:00.000Z",
    refreshing: false,
    cached: true,
    home_confirmation_required: false,
    failed_roots: [],
  });
  await page.route("**/api/v1/workspaces/*/repositories/discovery**", async (route) => {
    await route.fulfill({ status: 200, contentType: "application/json", body });
  });
  return repositories;
}

export async function openCreateProjectDialog(
  page: Page,
  pointer: "mouse" | "touch" = "mouse",
): Promise<Locator> {
  await page.goto("/office/projects");
  const newProject = page.getByRole("button", { name: /New Project|Create your first project/ });
  await expect(newProject.first()).toBeVisible({ timeout: 10_000 });
  await tapOrClick(newProject.first(), pointer);
  const dialog = page.getByTestId("create-project-dialog");
  await expect(dialog).toBeVisible();
  return dialog;
}

export function repositoryPickerTrigger(dialog: Locator): Locator {
  return dialog.getByTestId("project-add-repository");
}

/**
 * Resolves the open picker surface from the trigger's `aria-controls`, falling
 * back to the single mounted popover. The picker is portalled, so a descendant
 * lookup scoped to the dialog is not reliable.
 */
export async function resolveRepositoryPicker(trigger: Locator): Promise<Locator> {
  const page = trigger.page();
  const controls = await trigger.getAttribute("aria-controls");
  if (controls) {
    const byId = page.locator(`[id="${controls}"]`);
    if ((await byId.count()) > 0) return byId.first();
  }
  return page.locator('[data-slot="popover-content"]').first();
}

export async function openRepositoryPicker(
  trigger: Locator,
  pointer: "mouse" | "touch" = "mouse",
): Promise<Locator> {
  await tapOrClick(trigger, pointer);
  const picker = await resolveRepositoryPicker(trigger);
  await expect(picker).toBeVisible();
  await waitForFiniteAnimations(picker);
  return picker;
}

export function pickerSearchInput(picker: Locator): Locator {
  return picker.getByPlaceholder(/Search or paste a URL/i);
}

export function pickerCommandList(picker: Locator): Locator {
  return picker.locator('[data-slot="command-list"]');
}

export function pickerOptions(picker: Locator): Locator {
  return picker.getByRole("option");
}

/** Adds a free-form repository through the picker's custom-entry row. */
export async function addCustomRepository(
  dialog: Locator,
  value: string,
  pointer: "mouse" | "touch" = "mouse",
): Promise<void> {
  await tapOrClick(repositoryPickerTrigger(dialog), pointer);
  const picker = await resolveRepositoryPicker(repositoryPickerTrigger(dialog));
  await expect(picker).toBeVisible();
  const searchInput = pickerSearchInput(picker);
  await expect(searchInput).toBeVisible();
  await searchInput.fill(value);
  const customRow = picker.getByTestId("project-add-custom");
  await expect(customRow).toBeVisible({ timeout: 5_000 });
  await tapOrClick(customRow, pointer);
  await expect(picker).toHaveCount(0);
}

export async function tapOrClick(locator: Locator, pointer: "mouse" | "touch"): Promise<void> {
  if (pointer === "touch") {
    await locator.tap();
    return;
  }
  await locator.click();
}

/**
 * Asserts the picker's painted surface is fully inside the viewport. A clipped
 * ancestor can still report a layout box, so this is only half of the
 * regression: the caller also hit-tests an option beyond the dialog edge.
 */
export async function expectRepositoryPickerWithinViewport(
  picker: Locator,
  label = "repository picker",
): Promise<void> {
  const box = await picker.boundingBox();
  expect(box, `${label}: picker has no bounding box`).not.toBeNull();
  if (!box) return;
  const viewport = picker.page().viewportSize();
  expect(viewport, `${label}: page has no viewport`).not.toBeNull();
  if (!viewport) return;
  expect(
    box.x,
    `${label}: left edge ${box.x.toFixed(1)} is outside the viewport`,
  ).toBeGreaterThanOrEqual(0);
  expect(
    box.x + box.width,
    `${label}: right edge ${(box.x + box.width).toFixed(1)} exceeds viewport ${viewport.width}`,
  ).toBeLessThanOrEqual(viewport.width);
  expect(
    box.y,
    `${label}: top edge ${box.y.toFixed(1)} is outside the viewport`,
  ).toBeGreaterThanOrEqual(0);
  expect(
    box.y + box.height,
    `${label}: bottom edge ${(box.y + box.height).toFixed(1)} exceeds viewport ${viewport.height}`,
  ).toBeLessThanOrEqual(viewport.height);
}
