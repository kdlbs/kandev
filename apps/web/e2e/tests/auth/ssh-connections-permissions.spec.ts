import { randomUUID } from "node:crypto";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { expect } from "@playwright/test";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.5
test("only administrators can read or manage SSH identities and connections", async ({
  backend,
  browser,
  context,
  page,
}) => {
  const release = await backend.useEnv({
    KANDEV_FEATURES_AUTH: "true",
    KANDEV_FEATURES_SSH_CONNECTION_MANAGER: "true",
    KANDEV_DATABASE_PATH: path.join(backend.tmpDir, `ssh-access-${randomUUID()}.db`),
  });
  const admin = await browser.newContext({ baseURL: backend.frontendUrl });
  const root = `${backend.baseUrl}/api/v1/ssh/managed`;
  try {
    const credentials = { email: "ssh-admin@e2e.dev", password: "disposable-admin-123" };
    await setupAdmin(admin, backend.baseUrl, credentials);
    await login(admin, backend.baseUrl, credentials);
    await acceptInvite(context, backend.baseUrl, await createInviteToken(admin, backend.baseUrl), {
      email: "ssh-member@e2e.dev",
      password: "disposable-member-123",
    });
    const created = await admin.request.post(`${root}/identities`, {
      data: { request_id: randomUUID(), kind: "generate", label: "Administrator SSH key" },
    });
    expect(created.ok()).toBe(true);
    const key = (await created.json()) as { id: string; version: number };
    const connection = await admin.request.post(`${root}/connections`, {
      data: {
        request_id: randomUUID(),
        expected_version: 0,
        connection: {
          name: "Administrator target",
          host: "target.invalid",
          port: 22,
          user: "deploy",
          identity_id: key.id,
        },
      },
    });
    expect(connection.ok()).toBe(true);
    const route = (await connection.json()) as { destination: { id: string }; stamp: string };
    for (const endpoint of [
      "identities",
      `identities/${key.id}`,
      "connections",
      `connections/${route.destination.id}`,
    ]) {
      expect((await context.request.get(`${root}/${endpoint}`)).status()).toBe(403);
    }
    expect(
      (
        await context.request.post(`${root}/identities`, {
          data: { request_id: randomUUID(), kind: "generate", label: "Member attempt" },
        })
      ).status(),
    ).toBe(403);
    expect(
      (
        await context.request.delete(`${root}/identities/${key.id}`, {
          data: { expected_version: key.version },
        })
      ).status(),
    ).toBe(403);
    for (const action of ["observe", "preflight", "test", "trust", "enroll"]) {
      expect(
        (
          await context.request.post(`${root}/connections/${route.destination.id}/${action}`, {
            data: { expected_stamp: route.stamp },
          })
        ).status(),
      ).toBe(403);
    }
    for (const path of ["/settings/ssh-connections", "/settings/executors/ssh-connections"]) {
      await page.goto(`${backend.frontendUrl}${path}`);
      await expect(
        page.getByText("SSH connection management is not available.", { exact: true }),
      ).toBeVisible();
    }
    await expect(page.getByRole("button", { name: "New identity", exact: true })).toHaveCount(0);
    await expect(page.getByText("Administrator SSH key", { exact: true })).toHaveCount(0);
    const adminPage = await admin.newPage();
    await adminPage.goto(`/settings/ssh-connections?tab=identities&id=${key.id}`);
    await expect(
      adminPage.getByRole("heading", { name: "Administrator SSH key", exact: true }),
    ).toBeVisible();
    await expect(adminPage.getByLabel("Name", { exact: true })).toHaveCount(0);
    await expect(
      adminPage.getByRole("button", { name: "Copy public key", exact: true }),
    ).toBeVisible();
    await adminPage.getByRole("button", { name: "Edit", exact: true }).click();
    await expect(adminPage.getByLabel("Name", { exact: true })).toHaveValue(
      "Administrator SSH key",
    );
    const remaining = await admin.request.get(`${root}/identities`);
    expect(((await remaining.json()) as { id: string }[]).map((item) => item.id)).toEqual([key.id]);
  } finally {
    await page.close();
    await admin.close();
    await release();
  }
});
