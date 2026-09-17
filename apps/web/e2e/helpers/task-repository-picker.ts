import { expect, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "./animations";

export type TaskRepositoryPickerOptions = {
  mobile?: boolean;
  provider?: string;
};

async function chooseRepositorySource(page: Page, mobile: boolean): Promise<void> {
  const options = page.getByTestId("workspace-source-menu-options");
  await expect(options).toBeVisible();
  const repository = options.getByTestId("workspace-source-menu-repository");
  if (mobile) {
    await waitForFiniteAnimations(options);
    await repository.dispatchEvent("click");
  } else await repository.click();
}

/** Opens the shared task repository picker in the current responsive layout. */
export async function openTaskRepositoryPicker(
  page: Page,
  { mobile = false, provider }: TaskRepositoryPickerOptions = {},
): Promise<void> {
  if (mobile) {
    const add = page.getByTestId("mobile-repository-add");
    const management = page.getByTestId("mobile-repository-management");
    const sheet = page.getByTestId("mobile-repository-sheet-content");
    const sheetOpen = (await sheet.count()) > 0 && (await sheet.isVisible().catch(() => false));
    if (sheetOpen) {
      await waitForFiniteAnimations(sheet);
      if (!(await management.isVisible().catch(() => false))) {
        await expect
          .poll(() => sheet.isVisible().catch(() => false), {
            timeout: 10_000,
            message: "mobile repository sheet did not finish closing",
          })
          .toBe(false);
      }
    }
    if (!(await management.isVisible().catch(() => false))) {
      const manager = page.getByTestId("mobile-repository-manager");
      await expect(manager).toBeVisible({ timeout: 15_000 });
      await manager.tap();
      await expect(sheet).toBeVisible();
      await waitForFiniteAnimations(sheet);
    }
    await expect(management).toBeVisible();
    await expect(add).toBeVisible();
    await waitForFiniteAnimations(sheet);
    await add.dispatchEvent("click");
  } else {
    await page.getByTestId("add-repository").click();
  }

  await chooseRepositorySource(page, mobile);
  await expect(page.getByTestId("task-repository-picker")).toBeVisible();
  if (provider) {
    const sourceTab = page.getByTestId(`task-repository-source-${provider}`);
    await expect(sourceTab).toBeVisible({ timeout: 15_000 });
    if (mobile) {
      await waitForFiniteAnimations(page.getByTestId("task-repository-picker"));
      await sourceTab.dispatchEvent("click");
    } else await sourceTab.click();
  }
}

/** Selects a repository from the active source in the shared picker. */
export async function selectTaskRepository(
  page: Page,
  fullName: string,
  options: TaskRepositoryPickerOptions = {},
): Promise<void> {
  await openTaskRepositoryPicker(page, options);
  const option = page.getByTestId("task-repository-remote-option").filter({ hasText: fullName });
  await expect(option).toBeVisible({ timeout: 15_000 });
  if (options.mobile) await option.first().tap();
  else await option.first().click();
}

/** Adds a pasted repository URL through the shared picker. */
export async function pasteTaskRepositoryURL(
  page: Page,
  url: string,
  options: TaskRepositoryPickerOptions = {},
): Promise<void> {
  await openTaskRepositoryPicker(page, options);
  const input = page.getByTestId("task-repository-picker-input");
  await expect(input).toBeVisible();
  await input.fill(url);
  await input.press("Enter");
}
