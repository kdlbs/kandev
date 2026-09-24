import { expect, type Page } from "@playwright/test";
import { waitForFiniteAnimations } from "./animations";

/** Open the task workspace tools without toggling an already-open disclosure. */
export async function openTaskTools(page: Page) {
  const trigger = page.getByTestId("task-tools-trigger");
  await expect(trigger).toBeVisible();
  if ((await trigger.getAttribute("aria-expanded")) !== "true") await trigger.click();
  // Nested modal pickers hide their parent from the accessibility tree.
  const surface = page.getByRole("dialog", {
    name: "Task tools",
    exact: true,
    includeHidden: true,
  });
  await expect(surface).toBeVisible();
  await waitForFiniteAnimations(surface);
}
