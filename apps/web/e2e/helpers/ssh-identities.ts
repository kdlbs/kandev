import { generateKeyPairSync, randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { SSHIdentity } from "../../lib/types/http-ssh-connections";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";
import { exerciseSSHEnrollmentConfirmation } from "./ssh-enrollment";

// Only disposable keys are created here. These keys never authorize access to a host.
export async function exerciseSSHIdentitySetup(page: Page, baseUrl: string, touch: boolean) {
  const generatedName = `Generated browser key ${randomUUID()}`;
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/settings/executors/ssh-connections?tab=identities&new=1");
  await page.getByLabel("Name", { exact: true }).fill(generatedName);
  const create = page.getByRole("button", { name: "Create identity", exact: true });
  const height = (await requireBox(create, "create identity")).height;
  if (touch) expect(height).toBeGreaterThanOrEqual(44);
  else expect(height).toBeCloseTo(28, 0);
  await create.click();
  await expect(page).toHaveURL(/id=/);
  const generated = await selectedIdentity(page, baseUrl);
  expect(generated.public_key.startsWith("ssh-ed25519 ")).toBe(true);
  await page.getByRole("button", { name: "Copy public key", exact: true }).click();
  await expect(page.getByRole("status").filter({ hasText: "Public key copied." })).toBeVisible();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(generated.public_key);
  await assertNoDocumentHorizontalOverflow(page);

  const passphrase = "disposable-browser-passphrase";
  const { privateKey } = generateKeyPairSync("rsa", {
    modulusLength: 2048,
    privateKeyEncoding: { type: "pkcs1", format: "pem", cipher: "aes-256-cbc", passphrase },
    publicKeyEncoding: { type: "spki", format: "pem" },
  });
  const importedName = `Imported browser key ${randomUUID()}`;
  await page.goto("/settings/executors/ssh-connections?tab=identities&new=1");
  await page.getByLabel("Name", { exact: true }).fill(importedName);
  await page.getByLabel("Identity source", { exact: true }).selectOption("import");
  const material = page.getByLabel("Private key", { exact: true });
  const password = page.getByLabel("Key passphrase", { exact: true });
  await material.fill(privateKey);
  await password.fill("wrong-fixture-passphrase");
  await create.click();
  await expect(
    page.getByRole("alert").filter({ hasText: "Check the key, passphrase" }),
  ).toBeVisible();
  await expect(material).toHaveValue("");
  await expect(password).toHaveValue("");
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue(importedName);
  expect((await listIdentities(baseUrl)).some((item) => item.label === importedName)).toBe(false);

  await material.fill(privateKey);
  await password.fill(passphrase);
  await create.click();
  await expect(page).toHaveURL(/id=/);
  const imported = await selectedIdentity(page, baseUrl);
  expect(imported.kind).toBe("managed");
  expect(imported.public_key.startsWith("ssh-rsa ")).toBe(true);
  expect(JSON.stringify(imported).includes("PRIVATE KEY")).toBe(false);
  await expect(page.getByRole("heading", { name: importedName, exact: true })).toBeVisible();
  await expect(material).toHaveCount(0);
  await expect(password).toHaveCount(0);
  await assertNoDocumentHorizontalOverflow(page);

  await page.goto("/settings/executors/ssh-connections?tab=identities&new=1");
  await page.getByLabel("Identity source", { exact: true }).selectOption("import");
  await material.fill(privateKey);
  await password.fill(passphrase);
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page.getByRole("button", { name: "New identity", exact: true }).click();
  await page.getByLabel("Identity source", { exact: true }).selectOption("import");
  await expect(material).toHaveValue("");
  await expect(password).toHaveValue("");
  await exerciseSSHEnrollmentConfirmation(page, generated);
}

async function listIdentities(baseUrl: string): Promise<SSHIdentity[]> {
  const response = await fetch(`${baseUrl}/api/v1/ssh/managed/identities`);
  expect(response.ok).toBe(true);
  return response.json() as Promise<SSHIdentity[]>;
}
async function selectedIdentity(page: Page, baseUrl: string) {
  const id = new URL(page.url()).searchParams.get("id");
  const identity = (await listIdentities(baseUrl)).find((item) => item.id === id);
  expect(identity).toBeDefined();
  return identity!;
}
