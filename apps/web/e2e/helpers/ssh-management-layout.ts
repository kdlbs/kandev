import { randomUUID } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export async function seedSSHLayout(baseUrl: string) {
  const post = async (path: string, body: unknown) => {
    const response = await fetch(`${baseUrl}/api/v1/ssh/managed${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    expect(response.ok).toBe(true);
    return response.json();
  };
  const identity = await post("/identities", {
    request_id: randomUUID(),
    kind: "generate",
    label: "Layout identity",
  });
  const route = await post("/connections", {
    request_id: randomUUID(),
    expected_version: 0,
    connection: {
      name: "Layout connection",
      host: "layout.invalid",
      port: 22,
      user: "deploy",
      identity_id: identity.id,
    },
  });
  return { identity, route };
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-004.8, 004.9, 004.12
export async function exerciseSSHLayout(page: Page, baseUrl: string, mobile: boolean) {
  const { route } = await seedSSHLayout(baseUrl);
  await page.goto(`/settings/executors/ssh-connections?tab=connections&id=${route.destination.id}`);
  await page.getByRole("heading", { name: "Layout connection", exact: true }).waitFor();
  // Saved details must begin as a readable overview, not an edit form.
  await expect(page.getByLabel("Host", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Layout connection", exact: true })).toBeVisible();
  await expect(page.getByText("deploy@layout.invalid:22", { exact: true })).toBeVisible();
  await expect(page.getByLabel("One-time password", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Delete connection", exact: true })).toHaveCount(0);
  const edit = page.getByRole("button", { name: "Edit", exact: true });
  const box = await edit.boundingBox();
  expect(box).not.toBeNull();
  if (mobile) expect(box!.height).toBeGreaterThanOrEqual(44);
  else expect(Math.abs(box!.height - 28)).toBeLessThanOrEqual(1);
  await page.screenshot({ path: test.info().outputPath("ssh-overview.png"), fullPage: true });
  await edit.click();
  await expect(page.getByLabel("Host", { exact: true })).toHaveValue("layout.invalid");
  await page.getByLabel("Name", { exact: true }).fill("Renamed layout connection");
  const saved = page.waitForResponse(
    (r) =>
      r.url().includes(`/connections/${route.destination.id}`) && r.request().method() === "PATCH",
  );
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  expect((await saved).ok()).toBe(true);
  await expect(page.getByRole("tab", { name: "Access", exact: true })).toHaveAttribute(
    "data-state",
    "active",
  );
  await page.getByRole("tab", { name: "Overview", exact: true }).click();
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Renamed layout connection", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Back to connections", exact: true }).click();
  const row = page.locator(`a[id="ssh-row-${route.destination.id}"]`);
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("deploy@layout.invalid:22");
  await expect(row).toBeFocused();
  if (await page.evaluate(() => navigator.maxTouchPoints > 0)) await row.tap();
  else await row.press("Enter");
  await expect(
    page.getByRole("heading", { name: "Renamed layout connection", exact: true }),
  ).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
}

export async function exerciseSSHCollection(page: Page, baseUrl: string) {
  const { route } = await seedSSHLayout(baseUrl);
  const response = await fetch(`${baseUrl}/api/v1/ssh/managed/connections`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      request_id: randomUUID(),
      expected_version: 0,
      connection: { ...route.destination, id: undefined, version: 0, revision: 0 },
    }),
  });
  expect(response.ok).toBe(true);
  const duplicate = await response.json();
  await page.goto("/settings/executors/ssh-connections");
  const rows = page.getByRole("link").filter({ hasText: "Layout connection" });
  await expect(rows.filter({ hasText: route.destination.id })).toHaveCount(1);
  await expect(rows.filter({ hasText: duplicate.destination.id })).toHaveCount(1);
  await expect(rows.filter({ hasText: route.destination.id })).toContainText(
    "deploy@layout.invalid:22",
  );
  await page.evaluate(() => localStorage.setItem("theme", "dark"));
  await page.reload();
  await expect(page.locator("html")).toHaveClass(/dark/);
  await assertNoDocumentHorizontalOverflow(page);
  await page.evaluate(() => localStorage.setItem("theme", "light"));
  await page.reload();
  await expect(page.locator("html")).not.toHaveClass(/dark/);
  await assertNoDocumentHorizontalOverflow(page);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo");
  await expect(rows.filter({ hasText: route.destination.id })).toHaveCount(1);
  await assertNoDocumentHorizontalOverflow(page);
}

export async function exerciseSSHIdentityRemoval(page: Page, baseUrl: string, touch: boolean) {
  const name = `Unused identity ${randomUUID()}`;
  const response = await fetch(`${baseUrl}/api/v1/ssh/managed/identities`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ request_id: randomUUID(), kind: "generate", label: name }),
  });
  expect(response.ok).toBe(true);
  const identity = await response.json();
  await page.goto(`/settings/executors/ssh-connections?tab=identities&id=${identity.id}`);
  await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
  const more = page.getByRole("button", { name: "More actions", exact: true });
  const activate = (locator: import("@playwright/test").Locator) =>
    touch ? locator.tap() : locator.click();
  await activate(more);
  await activate(page.getByRole("menuitem", { name: "Delete identity", exact: true }));
  const confirmation = page.getByRole("dialog", { name: "Delete identity", exact: true });
  await expect(confirmation).toBeVisible();
  await activate(confirmation.getByRole("button", { name: "Cancel", exact: true }));
  await expect(more).toBeFocused();
  await activate(more);
  await activate(page.getByRole("menuitem", { name: "Delete identity", exact: true }));
  const deleted = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname.endsWith(`/identities/${identity.id}`) &&
      r.request().method() === "DELETE",
  );
  await activate(confirmation.getByRole("button", { name: "Delete identity", exact: true }));
  expect((await deleted).ok()).toBe(true);
  await expect(page).not.toHaveURL(/id=/);
  await expect(page.getByRole("button", { name: "New identity", exact: true })).toBeFocused();
  await expect(page.getByRole("link").filter({ hasText: name })).toHaveCount(0);
}
