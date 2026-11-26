import path from "node:path";
import { expect, type BrowserContext } from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { installFixturePlugin, PLUGIN_ID } from "../../helpers/plugin-fixture";
import {
  readUsage,
  selectUsageView,
  usageFixture,
  waitForUsage,
} from "../../helpers/usage-history";
import {
  createUsageTenant,
  deleteAuthenticatedTask,
  retainedOrganizationRows,
  retainedOrganizationScopes,
  retainedScopeFootprint,
  seedAuthenticatedUsage,
  transferUsageWorkspace,
  USAGE_AUTH_DATABASE,
  type UsageTenant,
} from "../../helpers/usage-history-auth";

test.describe.serial("Retained reporting authorization", () => {
  test.describe.configure({ retries: 0 });
  let operator: BrowserContext;
  let alpha: UsageTenant;
  let beta: UsageTenant;
  let member: BrowserContext | undefined;

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_MULTI_TENANCY: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, USAGE_AUTH_DATABASE),
    });
    operator = await browser.newContext({ baseURL: backend.frontendUrl });
    const admin = {
      email: "usage-operator@e2e.dev",
      password: "operatorpass123",
      displayName: "Usage operator",
    };
    await setupAdmin(operator, backend.baseUrl, admin);
    await login(operator, backend.baseUrl, admin);
    const page = await operator.newPage();
    await installFixturePlugin(page);
    alpha = await createUsageTenant(browser, backend, operator, "usage-alpha", "31");
    beta = await createUsageTenant(browser, backend, operator, "usage-beta", "47");
  });

  test.afterAll(async ({ backend }) => {
    try {
      if (operator) {
        const deleted = await operator.request.delete(`/api/plugins/${PLUGIN_ID}`);
        expect(deleted.ok(), await deleted.text()).toBe(true);
      }
    } finally {
      await member?.close();
      await alpha?.context.close();
      await beta?.context.close();
      await operator?.close();
      await backend.restart();
    }
  });

  test("rejects workspace and binding forgery while keeping deleted-task amounts visible to their owner", async () => {
    await waitForUsage(alpha.page, alpha.workspace, "31");
    await waitForUsage(beta.page, beta.workspace, "47");
    const foreign = await beta.context.request.get(
      `/api/v1/workspaces/${alpha.workspace}/stats/token-usage?range=all&org_id=${alpha.org.id}`,
    );
    expect(foreign.status(), await foreign.text()).toBe(404);
    expect(
      (
        await operator.request.get(
          `/api/v1/workspaces/${alpha.workspace}/stats/token-usage?range=all`,
        )
      ).status(),
    ).toBe(404);
    const forged = await usageFixture<{ Results: { Status: string }[] }>(
      alpha.page,
      alpha.workspace,
      {
        operation: "report",
        subject_binding: beta.binding,
        item_id: "cross-tenant-forgery",
        revision: 1,
        model_id: "forged",
        tokens: { total_tokens: "9999" },
      },
    );
    expect(forged.Results).toHaveLength(1);
    expect(forged.Results[0].Status).toBe("rejected");
    await deleteAuthenticatedTask(alpha.page, alpha.task);
    const retained = await readUsage(alpha.page, alpha.workspace);
    expect(retained.totals.amounts_decimal?.total_tokens).toBe("31");
    expect(retained.rows[0].task_available).toBe(false);
    await alpha.page.goto(`/stats/token-usage?workspaceId=${alpha.workspace}&range=all`);
    const breakdown = await selectUsageView(alpha.page, "Tasks", false);
    await expect(breakdown.getByTestId("token-usage-row")).toContainText("Deleted task");
    await expect(breakdown.getByTestId("token-usage-row").locator("a")).toHaveCount(0);
    await expect(alpha.page.getByTestId("token-usage-summary")).toContainText("31");
  });

  test("a workspace transfer does not transfer the former organization's retained accounting", async ({
    backend,
  }) => {
    const transferred = await seedAuthenticatedUsage(alpha.page, "53");
    transferUsageWorkspace(backend, transferred.workspace, beta);
    expect(
      (
        await alpha.context.request.get(
          `/api/v1/workspaces/${transferred.workspace}/stats/token-usage?range=all`,
        )
      ).status(),
    ).toBe(404);
    const report = await readUsage(beta.page, transferred.workspace);
    expect(report.rows).toEqual([]);
    expect(report.has_history).toBe(false);
    expect(report.totals.total_tokens).toBeNull();
    const forged = await usageFixture<{ Results: { Status: string }[] }>(
      beta.page,
      transferred.workspace,
      {
        operation: "report",
        subject_binding: transferred.binding,
        item_id: "former-owner-binding",
        revision: 1,
        tokens: { total_tokens: "8888" },
      },
    );
    expect(forged.Results).toHaveLength(1);
    expect(forged.Results[0].Status).toBe("rejected");
    await beta.page.goto(`/stats/token-usage?workspaceId=${transferred.workspace}&range=all`);
    await expect(beta.page.getByTestId("token-usage-empty-history")).toBeVisible();
    await expect(beta.page.getByTestId("token-usage-summary")).toHaveCount(0);
  });

  test("disabling a member clears their previously visible retained report", async ({
    browser,
    backend,
  }) => {
    const credentials = {
      email: "usage-member@e2e.dev",
      password: "memberpass123",
      displayName: "Usage member",
    };
    const created = await alpha.context.request.post("/api/v1/users", {
      data: {
        email: credentials.email,
        password: credentials.password,
        display_name: credentials.displayName,
        role: "member",
      },
    });
    expect(created.status(), await created.text()).toBe(201);
    const { user } = (await created.json()) as { user: { id: string } };
    const granted = await alpha.context.request.put(
      `/api/v1/workspaces/${alpha.workspace}/members/${user.id}`,
      { data: { role: "collaborator" } },
    );
    expect(granted.status(), await granted.text()).toBe(200);
    member = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(member, backend.baseUrl, credentials);
    const page = await member.newPage();
    await waitForUsage(page, alpha.workspace, "31");
    await page.goto(`/stats/token-usage?workspaceId=${alpha.workspace}&range=all`);
    await expect(page.getByTestId("token-usage-summary")).toContainText("31");
    const disabled = await alpha.context.request.patch(`/api/v1/users/${user.id}`, {
      data: { status: "disabled" },
    });
    expect(disabled.status(), await disabled.text()).toBe(200);
    expect(
      (
        await member.request.get(
          `/api/v1/workspaces/${alpha.workspace}/stats/token-usage?range=all`,
        )
      ).status(),
    ).toBe(401);
    await page.reload();
    await expect(page.getByTestId("login-email")).toBeVisible();
    await expect(page.getByTestId("token-usage-summary")).toHaveCount(0);
  });

  test("suspension blocks saved data and organization erasure removes retained ownership without affecting another tenant", async ({
    backend,
  }) => {
    expect(retainedOrganizationRows(backend, alpha.org.id).count).toBeGreaterThan(0);
    const scopes = retainedOrganizationScopes(backend, alpha.org.id);
    expect(retainedScopeFootprint(backend, scopes).usage_canonical_facts).toBeGreaterThan(0);
    const suspended = await operator.request.patch(`/api/v1/instance/orgs/${alpha.org.id}`, {
      data: { status: "suspended" },
    });
    expect(suspended.status(), await suspended.text()).toBe(200);
    expect(
      (
        await alpha.context.request.get(
          `/api/v1/workspaces/${alpha.workspace}/stats/token-usage?range=all`,
        )
      ).status(),
    ).toBe(401);
    await alpha.page.reload();
    await expect(alpha.page.getByTestId("login-email")).toBeVisible();
    await expect(alpha.page.getByTestId("token-usage-summary")).toHaveCount(0);
    const erased = await operator.request.delete(`/api/v1/instance/orgs/${alpha.org.id}`, {
      data: { slug: alpha.org.slug },
    });
    expect(erased.status(), await erased.text()).toBe(200);
    expect(retainedOrganizationRows(backend, alpha.org.id).count).toBe(0);
    for (const count of Object.values(retainedScopeFootprint(backend, scopes)))
      expect(count).toBe(0);
    expect(
      (
        await beta.context.request.get(
          `/api/v1/workspaces/${alpha.workspace}/stats/token-usage?range=all`,
        )
      ).status(),
    ).toBe(404);
    await waitForUsage(beta.page, beta.workspace, "47");
    await beta.page.goto(`/stats/token-usage?workspaceId=${beta.workspace}&range=all`);
    await expect(beta.page.getByTestId("token-usage-summary")).toContainText("47");
  });
});
