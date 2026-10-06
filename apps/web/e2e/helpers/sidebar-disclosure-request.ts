import type { Page, Locator } from "@playwright/test";

function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

export async function heldDisclosure(page: Page, header: Locator, verify: () => Promise<void>) {
  const arrived = gate();
  const release = gate();
  const key = await header.getAttribute("data-group-key");
  const collapsing = (await header.getAttribute("aria-expanded")) === "true";
  const matches = (query: { collapsed_group_keys: string[] }) =>
    query.collapsed_group_keys.includes(key!) === collapsing;
  await page.route("**/sidebar/query", async (route) => {
    if (!matches(route.request().postDataJSON())) return route.continue();
    arrived.release();
    await release.promise;
    await route.continue();
  });
  const response = page.waitForResponse(
    (result) =>
      result.url().endsWith("/sidebar/query") &&
      result.request().method() === "POST" &&
      matches(result.request().postDataJSON()),
  );
  // The response waiter can outlive a failed assertion until the page closes.
  void response.catch(() => {});
  try {
    await header.click();
    await arrived.promise;
    await verify();
    release.release();
    await response;
  } catch (error) {
    release.release();
    await page.unrouteAll({ behavior: "wait" }).catch(() => {});
    throw error;
  }
  await page.unrouteAll({ behavior: "wait" });
}
