import { devices, type Page } from "@playwright/test";
import path from "node:path";
import { test, expect } from "../../fixtures/ssh-test-base";
import {
  startBastionAndTarget,
  stopBastionAndTarget,
  type SSHBastionHandles,
} from "../../helpers/ssh-bastion";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import type { SSHRoute } from "../../../lib/types/http-ssh-connections";

async function activate(page: Page, label: string, touch: boolean) {
  const button = page.getByRole("button", { name: label, exact: true });
  if (touch) await button.tap();
  else await button.click();
}
async function confirm(page: Page, label: string, touch: boolean) {
  await activate(page, label, touch);
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const action = dialog.getByRole("button", { name: label, exact: true });
  if (touch) await action.tap();
  else await action.click();
}
async function verifyAccess(page: Page, touch: boolean) {
  const response = page.waitForResponse(
    (r) => r.url().endsWith("/test") && r.request().method() === "POST",
  );
  await activate(page, "Test SSH access", touch);
  expect((await response).ok()).toBe(true);
  await expect(page.getByRole("status").filter({ hasText: "SSH access verified" })).toBeVisible();
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-002.7, AC-EXECUTORS-SSH-CONNECTIONS-004.4, AC-EXECUTORS-SSH-CONNECTIONS-005.2
for (const topology of ["direct", "bastion", "unpinned"] as const) {
  for (const touch of [false, true]) {
    test(`${touch ? "phone" : "desktop"} ${topology} adoption keeps trust and handles a concurrent edit`, async ({
      backend,
      apiClient,
      seedData,
      browser,
    }, testInfo) => {
      test.setTimeout(240_000);
      const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
      const context = await browser.newContext({
        ...(touch ? devices["Pixel 5"] : { viewport: { width: 1280, height: 900 } }),
        baseURL: backend.frontendUrl,
      });
      const page = await context.newPage();
      await page.addInitScript(
        ({ port }) => {
          localStorage.setItem("kandev.onboarding.completed", "true");
          window.__KANDEV_API_PORT = port;
        },
        { port: String(backend.port) },
      );
      let handles: SSHBastionHandles | undefined;
      try {
        if (topology === "bastion")
          handles = startBastionAndTarget(
            testInfo.workerIndex,
            path.join(backend.tmpDir, `browser-${touch}`),
          );
        const target = handles?.target ?? seedData.sshTarget;
        const config: Record<string, string> = {
          ssh_host: target.host,
          ssh_port: String(target.port),
          ssh_user: target.user,
          ssh_identity_source: "file",
          ssh_identity_file: target.identityFile,
          ssh_shell: "bash",
        };
        if (handles)
          config.ssh_proxy_jump = `${handles.bastion.user}@${handles.bastion.host}:${handles.bastion.port}`;
        const probe = await apiClient.testSSHConnection({
          name: "Browser adoption source",
          host: target.host,
          port: target.port,
          user: target.user,
          identity_source: "file",
          identity_file: target.identityFile,
          proxy_jump: config.ssh_proxy_jump,
        });
        expect(probe.success).toBe(true);
        if (topology !== "unpinned") config.ssh_host_fingerprint = probe.fingerprint!;
        const executor = await apiClient.createSSHExecutor("Browser legacy profile", config);
        await page.goto(`/settings/executors/ssh/${executor.id}`);
        await activate(page, "Move to a shared connection", touch);
        if (topology !== "unpinned")
          await expect(
            page
              .getByRole("button", { name: "Move to a shared connection", exact: true })
              .locator("..")
              .getByText(probe.fingerprint!, { exact: true }),
          ).toBeVisible();
        await activate(page, "Create connection drafts", touch);
        await page.getByRole("dialog").getByRole("button", { name: "Cancel", exact: true }).click();
        expect((await apiClient.getExecutor(executor.id)).config).toEqual(config);
        await confirm(page, "Create connection drafts", touch);
        await expect(page).toHaveURL(/adopt_connection=/);
        const destinationId = new URL(page.url()).searchParams.get("id")!;
        const destinationUrl = new URL(page.url());
        destinationUrl.searchParams.set("section", "access");
        await page.goto(destinationUrl.toString());
        const response = await fetch(
          `${backend.baseUrl}/api/v1/ssh/managed/connections/${destinationId}`,
        );
        expect(response.ok).toBe(true);
        const draft = (await response.json()) as SSHRoute;
        expect(draft.destination.fingerprint).toBe(config.ssh_host_fingerprint ?? "");
        expect((await apiClient.getExecutor(executor.id)).config).toEqual(config);
        await expect(
          page.getByRole("button", { name: "Use this connection", exact: true }),
        ).toBeDisabled();
        if (draft.bastion) {
          await page.goto(
            `/settings/executors/ssh-connections?tab=connections&id=${draft.bastion.id}&section=access`,
          );
          await activate(page, "Inspect host key", touch);
          await confirm(page, "Trust this host key", touch);
          await verifyAccess(page, touch);
          await page.goto(destinationUrl.toString());
        }
        if (topology === "unpinned") {
          await expect(
            page.getByRole("button", { name: "Test SSH access", exact: true }),
          ).toHaveCount(0);
          await activate(page, "Inspect host key", touch);
          await confirm(page, "Trust this host key", touch);
        }
        await verifyAccess(page, touch);
        const readyResponse = await fetch(
          `${backend.baseUrl}/api/v1/ssh/managed/connections/${destinationId}`,
        );
        expect(readyResponse.ok).toBe(true);
        const ready = (await readyResponse.json()) as SSHRoute;
        const concurrent = { ...config, ssh_shell: "sh" };
        await apiClient.updateExecutor(executor.id, { config: concurrent });
        await confirm(page, "Use this connection", touch);
        await expect(
          page.getByRole("alert").filter({ hasText: "Adoption could not proceed" }),
        ).toBeVisible();
        expect((await apiClient.getExecutor(executor.id)).config).toEqual(concurrent);
        await apiClient.updateExecutor(executor.id, { config });
        await page.reload();
        await confirm(page, "Use this connection", touch);
        await expect(page).toHaveURL(`/settings/executors/ssh/${executor.id}`);
        expect((await apiClient.getExecutor(executor.id)).config).toEqual({
          ssh_shell: "bash",
          ssh_connection_id: destinationId,
          ssh_connection_stamp: ready.stamp,
        });
        await page.reload();
        await expect(
          page.getByRole("button", { name: "Move to a shared connection", exact: true }),
        ).toHaveCount(0);
        await assertNoDocumentHorizontalOverflow(page);
      } finally {
        await context.close();
        if (handles) stopBastionAndTarget(handles);
        await release();
      }
    });
  }
}

for (const touch of [false, true]) {
  test(`${touch ? "phone" : "desktop"} unsupported jump chains preserve the legacy profile`, async ({
    backend,
    apiClient,
    seedData,
    browser,
  }) => {
    const release = await backend.useEnv({ KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true" });
    const context = await browser.newContext({
      ...(touch ? devices["Pixel 5"] : { viewport: { width: 1280, height: 900 } }),
      baseURL: backend.frontendUrl,
    });
    const page = await context.newPage();
    await page.addInitScript(
      ({ port }) => {
        localStorage.setItem("kandev.onboarding.completed", "true");
        window.__KANDEV_API_PORT = port;
      },
      { port: String(backend.port) },
    );
    try {
      const config = {
        ssh_host: seedData.sshTarget.host,
        ssh_port: String(seedData.sshTarget.port),
        ssh_user: seedData.sshTarget.user,
        ssh_identity_source: "file",
        ssh_identity_file: seedData.sshTarget.identityFile,
        ssh_host_fingerprint: seedData.sshTarget.hostFingerprint,
        ssh_proxy_jump: "first.invalid,second.invalid",
      };
      const executor = await apiClient.createSSHExecutor("Unsupported chain", config);
      const list = () =>
        fetch(`${backend.baseUrl}/api/v1/ssh/managed/connections`).then(async (r) => {
          expect(r.ok).toBe(true);
          return r.json();
        });
      const before = await list();
      await page.goto(`/settings/executors/ssh/${executor.id}`);
      await activate(page, "Move to a shared connection", touch);
      await expect(
        page.getByRole("alert").filter({ hasText: "unsupported jump chains" }),
      ).toBeVisible();
      await expect(
        page.getByRole("button", { name: "Create connection drafts", exact: true }),
      ).toHaveCount(0);
      expect((await apiClient.getExecutor(executor.id)).config).toEqual(config);
      expect(await list()).toEqual(before);
      await assertNoDocumentHorizontalOverflow(page);
    } finally {
      await context.close();
      await release();
    }
  });
}
