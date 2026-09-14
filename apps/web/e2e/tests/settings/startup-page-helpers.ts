import { expect, type Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";

export const VIEW_STORAGE_KEY = "kandev.taskListing.view.v1";

export async function saveStartupChoice(page: Page, touch = false) {
  const floatingSave = page.getByTestId("settings-floating-save");
  const button = floatingSave.getByRole("button", { name: /^(Save changes|Retry save)$/ });
  const saved = waitForHttp(page, "PATCH", /\/api\/v1\/user\/settings$/, {
    predicate: (response) => response.request().postDataJSON()?.startup_page === "threads",
  });
  if (touch) await button.tap();
  else await button.click();
  expect((await saved).status()).toBe(200);
  await expect(floatingSave).not.toBeVisible();
}

export async function expectThreadsHome(page: Page, workspaceId: string) {
  await expect(page).toHaveURL(
    (url) => url.pathname === "/threads" && url.searchParams.get("workspace") === workspaceId,
  );
  await expect(
    page.getByTestId("threads-board").or(page.getByTestId("threads-empty-state")),
  ).toBeVisible();
}

export async function expectEmptyList(page: Page) {
  await expect(page).toHaveURL((url) => url.pathname === "/tasks");
  await expect(page.getByText("No tasks found.", { exact: true })).toBeVisible();
}
