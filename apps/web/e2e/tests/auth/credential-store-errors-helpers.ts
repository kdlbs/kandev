import { expect, type Page } from "@playwright/test";
import { waitForHttp } from "../../helpers/causal-waits";

export type CredentialStoreAccount = {
  email: string;
  password: string;
};

const UNAVAILABLE_BODY = JSON.stringify({
  error: "authentication temporarily unavailable",
  code: "persistence_unavailable",
});

async function sessionCookie(page: Page): Promise<{ name: string; value: string }> {
  const cookies = await page.context().cookies();
  const session = cookies.find((cookie) => cookie.name.startsWith("kandev_session"));
  expect(session, "the browser must hold a Kandev session cookie").toBeTruthy();
  return { name: session?.name ?? "", value: session?.value ?? "" };
}

async function openTokenSurface(page: Page) {
  await page.goto("/settings/account/tokens");
  await expect(page.getByTestId("api-tokens-card")).toBeVisible();
}

async function armTokenReadResponse(page: Page, status: 401 | 503) {
  let observedRequest: { method: string; url: string } | null = null;
  const headers: Record<string, string> =
    status === 401 ? { "WWW-Authenticate": "Bearer" } : { "Content-Type": "application/json" };
  await page.route("**/api/v1/auth/tokens", async (route) => {
    if (route.request().method() !== "GET") {
      await route.continue();
      return;
    }
    observedRequest = { method: route.request().method(), url: route.request().url() };
    const body =
      status === 401 ? JSON.stringify({ error: "authentication required" }) : UNAVAILABLE_BODY;
    await route.fulfill({
      status,
      headers,
      body,
    });
  });
  return () => observedRequest;
}

/**
 * Proves that an authentication-storage 503 keeps the current browser session
 * intact, then that the same surface recovers without a login. Ends with a
 * challenged-401 control that still reaches the sign-in page.
 */
export async function proveCredentialStoreContinuity(
  page: Page,
  account: CredentialStoreAccount,
): Promise<void> {
  await openTokenSurface(page);
  await expect(page.getByTestId("api-tokens-empty")).toBeVisible();
  const cookieBefore = await sessionCookie(page);

  const tokenRead = await armTokenReadResponse(page, 503);
  const unavailableResponse = waitForHttp(page, "GET", /^\/api\/v1\/auth\/tokens$/, {
    predicate: (response) => response.status() === 503,
  });
  await page.reload();
  await unavailableResponse;

  const observed = tokenRead();
  expect(observed?.method).toBe("GET");
  expect(observed?.url).toContain("/api/v1/auth/tokens");
  await expect(page.getByTestId("api-tokens-error")).toHaveText(
    "authentication temporarily unavailable",
  );
  expect(new URL(page.url()).pathname).toBe("/settings/account/tokens");
  await expect(page.getByTestId("current-user-chip")).toHaveAttribute("aria-label", account.email);
  expect(await sessionCookie(page)).toEqual(cookieBefore);

  await page.unroute("**/api/v1/auth/tokens");
  await page.reload();
  await expect(page.getByTestId("api-tokens-card")).toBeVisible();
  await expect(page.getByTestId("api-tokens-empty")).toBeVisible();
  await expect(page.getByTestId("api-tokens-error")).toHaveCount(0);
  expect(new URL(page.url()).pathname).toBe("/settings/account/tokens");
  await expect(page.getByTestId("current-user-chip")).toHaveAttribute("aria-label", account.email);
  expect(await sessionCookie(page)).toEqual(cookieBefore);

  await armTokenReadResponse(page, 401);
  const challengedResponse = waitForHttp(page, "GET", /^\/api\/v1\/auth\/tokens$/, {
    predicate: (response) => response.status() === 401,
  });
  await page.reload();
  await challengedResponse;
  await expect(page).toHaveURL(/\/login/);
}
