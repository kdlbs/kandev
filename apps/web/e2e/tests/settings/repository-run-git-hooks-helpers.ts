import { expect, type Page, type Route } from "@playwright/test";

export async function openRepositoryCard(page: Page, workspaceId: string) {
  await page.goto(`/settings/workspace/${workspaceId}/repositories`);
  await page.getByRole("heading", { name: "E2E Repo", exact: true }).click();
  const field = page.getByTestId("repository-run-git-hooks");
  await field.scrollIntoViewIfNeeded();
  await expect(field).toBeVisible();
  return field.getByTestId("repository-run-git-hooks-switch");
}

export async function saveSettings(page: Page) {
  const save = page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: /Save changes|Retry save/ });
  await expect(save).toBeEnabled();
  const updated = page.waitForResponse(
    (response) =>
      response.request().method() === "PATCH" &&
      /\/api\/v1\/repositories\/[^/?]+$/.test(response.url()),
  );
  await save.click();
  expect((await updated).ok()).toBe(true);
  await expect(save).toBeHidden();
}

const REPOSITORY_UPDATE_URL = /\/api\/v1\/repositories\/[^/?]+$/;

function isRepositoryUpdate(route: Route) {
  return route.request().method() === "PATCH";
}

export function saveButton(page: Page) {
  return page
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: /Save changes|Retry save/ });
}

/** Fails every repository PATCH with a 500 until the returned function is called. */
export async function failRepositoryUpdates(page: Page) {
  const handler = (route: Route) =>
    isRepositoryUpdate(route)
      ? route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "forced failure" }),
        })
      : route.fallback();
  await page.route(REPOSITORY_UPDATE_URL, handler);
  return () => page.unroute(REPOSITORY_UPDATE_URL, handler);
}

/** Holds the first repository PATCH until release() is called, then forwards it. */
export async function holdRepositoryUpdate(page: Page) {
  let release: () => void = () => {};
  const gate = new Promise<void>((resolve) => {
    release = resolve;
  });
  let heldResolve: () => void = () => {};
  const held = new Promise<void>((resolve) => {
    heldResolve = resolve;
  });
  let first = true;
  await page.route(REPOSITORY_UPDATE_URL, async (route) => {
    if (!isRepositoryUpdate(route) || !first) return route.fallback();
    first = false;
    heldResolve();
    await gate;
    await route.fallback();
  });
  return { held, release };
}
