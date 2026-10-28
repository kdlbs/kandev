import { expect, test, type Page } from "@playwright/test";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";

/** Real backend metadata/identity setup. No host authentication or enrollment. */
export async function exerciseSSHContextSetup(page: Page, touch: boolean) {
  const suffix = Date.now().toString();
  const name = `Unfinished executor ${suffix}`;
  const target = `Target ${suffix}`;
  const identity = `Identity ${suffix}`;
  await page.goto("/settings/executors/new/ssh");
  await page.getByLabel("Name", { exact: true }).fill(name);
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  await expect(page.getByRole("button", { name: "Return to executor", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "New connection", exact: true }).click();
  await page.getByLabel("Name", { exact: true }).fill(target);
  await page.getByLabel("Host", { exact: true }).fill("fixture.invalid");
  await page.getByLabel("SSH user", { exact: true }).fill("builder");
  const create = page.getByRole("button", { name: "Create identity", exact: true });
  expect(
    (await requireBox(create, "identity creation action")).height + 0.01,
  ).toBeGreaterThanOrEqual(touch ? 44 : 28);
  await create.click();
  await expect(page).toHaveURL(/return_identity_connection=/);
  await page.goBack();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue(target);
  await create.click();
  await page.getByLabel("Name", { exact: true }).fill("Cancelled identity");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue(target);
  await expect(page.getByLabel("Host", { exact: true })).toHaveValue("fixture.invalid");
  await create.click();
  await page.getByLabel("Name", { exact: true }).fill(identity);
  await page.getByRole("button", { name: "Create identity", exact: true }).click();
  const picker = page.getByLabel("Identity", { exact: true });
  await expect(picker).toBeVisible();
  if (touch) await expect(picker).toHaveText(identity);
  else await expect(picker.locator("option:checked")).toHaveText(identity);
  await expect(page).not.toHaveURL(/select_identity=/);
  await expect(picker).toBeFocused();
  await page.getByRole("button", { name: "Save changes", exact: true }).click();
  await page.getByRole("button", { name: "Return to executor", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue(name);
  await expect(page.getByLabel("Shared connection", { exact: true })).toBeFocused();
  await expect(page.getByRole("button", { name: "Test executor", exact: true })).toBeDisabled();
  await assertNoDocumentHorizontalOverflow(page);
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  await page.getByRole("link").filter({ hasText: target }).click();
  await page.reload();
  await expect(page.getByText("builder@fixture.invalid:22", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "Return to executor", exact: true })).toHaveCount(
    0,
  );
  await assertNoDocumentHorizontalOverflow(page);
}

/** UI-only ready-route and executor-test fixtures; this does not prove SSH authentication. */
export async function exerciseSSHExplicitSelection(page: Page, touch: boolean) {
  const destination = {
    id: "ready-ui-fixture",
    name: "Ready UI fixture",
    host: "fixture.invalid",
    port: 22,
    user: "builder",
    identity_id: "public-fixture",
    fingerprint: "public-pin",
    public_key: "",
    version: 1,
    revision: 1,
    verified_route: "ready-ui-stamp",
  };
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() !== "GET" && /\/api\/v1\/(executors|ssh\/managed)/.test(request.url()))
      writes.push(request.url());
  });
  await page.route("**/api/v1/ssh/managed/connections", (route) =>
    route.fulfill({ json: [{ destination, stamp: "ready-ui-stamp" }] }),
  );
  await page.route("**/api/v1/ssh/test", (route) => route.fulfill({ json: { success: true } }));
  await page.goto("/settings/executors/new/ssh");
  await page.getByLabel("Name", { exact: true }).fill("Preserved executor");
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  await page.goBack();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Preserved executor");
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  await page.getByRole("link").filter({ hasText: destination.name }).click();
  const use = page.getByRole("button", { name: "Use connection", exact: true });
  expect((await requireBox(use, "Use connection")).height + 0.01).toBeGreaterThanOrEqual(
    touch ? 44 : 28,
  );
  await use.click();
  const picker = page.getByLabel("Shared connection", { exact: true });
  if (touch) await expect(picker).toHaveText(destination.name);
  else await expect(picker).toHaveValue(destination.id);
  await expect(picker).toBeFocused();
  const save = page.getByRole("button", { name: "Save", exact: true });
  await expect(save).toBeDisabled();
  await page.getByRole("button", { name: "Test executor", exact: true }).click();
  await expect(save).toBeEnabled();
  expect(writes).toEqual([]);
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  await page.getByRole("button", { name: "Return to executor", exact: true }).click();
  await expect(save).toBeDisabled();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Preserved executor");
  await assertNoDocumentHorizontalOverflow(page);
}

/** Read-only executor metadata fixture for concurrent-editor recovery. */
export async function exerciseSSHConcurrentEdit(page: Page) {
  let name = "Original fixture executor";
  const executorId = "concurrent-ui-fixture";
  await page.route(`**/api/v1/executors/${executorId}`, (route) =>
    route.fulfill({
      json: {
        id: executorId,
        name,
        type: "ssh",
        config: { ssh_connection_id: "public-fixture", ssh_connection_stamp: "fixture-stamp" },
      },
    }),
  );
  await page.goto(`/settings/executors/ssh/${executorId}`);
  await page.getByLabel("Name", { exact: true }).fill("My unsaved executor");
  await page.getByRole("button", { name: "Manage connections", exact: true }).click();
  name = "Changed by another administrator";
  await page.getByRole("button", { name: "Return to executor", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue("My unsaved executor");
  await expect(
    page.getByText(
      "The executor settings changed while you were away. Discard this draft to load the current settings.",
      { exact: true },
    ),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  await page
    .getByRole("button", { name: "Discard draft and reload", exact: true })
    .evaluate((node) => node.scrollIntoView({ block: "center" }));
  await page.screenshot({ path: test.info().outputPath("ssh-draft-conflict.png") });
  await page.getByRole("button", { name: "Discard draft and reload", exact: true }).click();
  await expect(page.getByLabel("Name", { exact: true })).toHaveValue(name);
  await expect(page.getByTestId("settings-scroll-container")).toHaveCount(1);
  await assertNoDocumentHorizontalOverflow(page);
}
