import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { PrAssetCapture } from "./pr-asset-capture";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";

export async function exerciseSSHIdentityDependencies(
  page: Page,
  baseUrl: string,
  touch: boolean,
  capture?: PrAssetCapture,
) {
  const create = async (path: string, body: unknown) => {
    const response = await fetch(`${baseUrl}/api/v1/ssh/managed/${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    expect(response.ok).toBe(true);
    return response.json();
  };
  const identity = await create("identities", {
    request_id: randomUUID(),
    kind: "generate",
    label: "Deployment key",
  });
  const route = await create("connections", {
    request_id: randomUUID(),
    expected_version: 0,
    connection: {
      name: "Build server",
      host: "build.example.invalid",
      port: 22,
      user: "builder",
      identity_id: identity.id,
    },
  });
  await page.goto(`/settings/executors/ssh-connections?tab=identities&id=${identity.id}`);
  const dependency = page.getByRole("link", { name: "Connection: Build server", exact: true });
  await expect(dependency).toBeVisible();
  await page.getByRole("button", { name: "More actions", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Delete identity", exact: true })).toBeDisabled();
  await page.keyboard.press("Escape");
  if (touch)
    expect((await requireBox(dependency, "dependency link")).height).toBeGreaterThanOrEqual(44);
  await assertNoDocumentHorizontalOverflow(page);
  await dependency.scrollIntoViewIfNeeded();
  await capture?.screenshot("identity-dependencies", {
    caption: "Identity deletion dependencies",
    fullPage: false,
  });
  if (touch) await dependency.tap();
  else await dependency.click();
  await expect(page).toHaveURL(new RegExp(`id=${route.destination.id}`));
  await expect(page.getByText("builder@build.example.invalid:22", { exact: true })).toBeVisible();
}
