import type { Locator, Page } from "@playwright/test";

/**
 * The office topbar's title crumb: the current-page segment of the shared
 * breadcrumb. Replaces the pre-contract `<h1>` the office shell used to
 * render, so specs assert the painted title without depending on markup.
 */
export function officeTopbarTitle(page: Page): Locator {
  return page.getByTestId("office-topbar").locator('[data-slot="breadcrumb-page"]');
}
