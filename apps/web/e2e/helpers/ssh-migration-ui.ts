import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { ApiClient } from "./api-client";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";
import { insertLegacySSHFixture, readSSHRoute } from "./ssh-managed-fixture";

export async function exerciseImportedSSHRepair(
  page: Page,
  backend: BackendContext,
  apiClient: ApiClient,
  touch: boolean,
) {
  const name = "Historical SSH connection";
  const key = path.join(backend.tmpDir, `migration-repair-${randomUUID()}`);
  execFileSync("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", key]);
  const { executorId, profileId } = insertLegacySSHFixture(backend.tmpDir, name, {
    ssh_host: "legacy.example.invalid",
    ssh_port: "2222",
    ssh_user: "builder",
    ssh_identity_source: "file",
    ssh_identity_file: key,
  });
  await backend.restart();

  const executor = await apiClient.getExecutor(executorId);
  const id = executor.config?.ssh_connection_id;
  expect(id).toBeTruthy();
  expect(executor.config?.ssh_host).toBeUndefined();
  const route = await readSSHRoute(backend.baseUrl, id!);
  await page.goto(`/settings/executors/${profileId}`);
  await page.getByTestId("ssh-connection-settings-link").click();
  await expect(page).toHaveURL(`/settings/executors/ssh/${executorId}`);
  await expect(
    page.getByText("builder@legacy.example.invalid:2222", { exact: true }),
  ).toBeVisible();
  const picker = page.getByLabel("Shared connection", { exact: true });
  if (touch) await expect(picker).toHaveText(route.destination.name);
  else await expect(picker).toHaveValue(id!);
  await expect(page.getByRole("button", { name: "Test executor", exact: true })).toBeDisabled();
  const review = page.getByRole("button", { name: "Review connection", exact: true });
  expect((await requireBox(review, "Review connection")).height + 0.01).toBeGreaterThanOrEqual(
    touch ? 44 : 28,
  );
  await review.click();
  const observe = page.getByRole("button", { name: "Inspect host key", exact: true });
  await expect(observe).toBeVisible();
  // The database and the imported route are real. Stub the remote observation only.
  const observedFingerprint = "SHA256:hiYklNSU5jGah1fEFQjbMDPzJku7HSJggitp7pYTAB0";
  await page.route(`**/api/v1/ssh/managed/connections/${id}/observe`, (request) =>
    request.fulfill({
      json: {
        receipt: "observation-fixture",
        stamp: route.stamp,
        fingerprint: observedFingerprint,
        public_key: "",
      },
    }),
  );
  await observe.click();
  const heading = page.getByRole("heading", { name: "Verify this server", exact: true });
  await expect(heading).toBeFocused();
  const fingerprint = page.getByText(observedFingerprint, { exact: true });
  await expect(fingerprint).toBeVisible();
  await assertNoDocumentHorizontalOverflow(page);
  if (touch) {
    await heading.evaluate((element) => {
      const scroll = element.closest('[data-testid="settings-scroll-container"]');
      if (!scroll) throw new Error("Missing settings scroll container");
      scroll.scrollTop +=
        element.getBoundingClientRect().top - scroll.getBoundingClientRect().top - 16;
    });
  }
  await expect(heading).toBeInViewport({ ratio: 1 });
  await expect(fingerprint).toBeInViewport({ ratio: 1 });
  const captureStyle = await page.addStyleTag({
    content: `
      [data-testid="toast-container"],
      [data-testid="toast-message"],
      [aria-label="Configuration Chat"] { display: none !important; }
    `,
  });
  try {
    await expect(page.getByTestId("toast-container")).toBeHidden();
    await page.screenshot({
      path: test.info().outputPath("ssh-migrated-access.png"),
      scale: "css",
    });
  } finally {
    await captureStyle.evaluate((element) => element.remove());
  }
  await page.getByRole("button", { name: "Back", exact: true }).click();
  await expect(observe).toBeFocused();
  expect((await readSSHRoute(backend.baseUrl, id!)).destination.fingerprint).toBe("");
  await page.getByRole("button", { name: "Return to executor", exact: true }).click();
  await expect(picker).toBeFocused();

  const oldLink = new URLSearchParams({
    id: id!,
    adopt_executor: executorId,
    adopt_connection: id!,
    adopt_digest: "obsolete",
  });
  await page.goto(`/settings/ssh-connections?${oldLink}`);
  await expect(observe).toBeVisible();
  await expect(page.getByRole("button", { name: "Use this connection", exact: true })).toHaveCount(
    0,
  );
}
