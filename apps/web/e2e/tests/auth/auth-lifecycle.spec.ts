import { expect } from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import {
  acceptInvite,
  createInviteToken,
  fetchBootState,
  login,
  mintPAT,
  setupAdmin,
} from "../../helpers/auth";

/**
 * Opt-in authentication lifecycle. Runs in the dedicated `auth` Playwright
 * project: the worker's backend is restarted with KANDEV_FEATURES_AUTH=true,
 * so it must never share a worker with the default suite (which assumes an
 * open backend). Serial: later tests depend on the admin created in setup.
 *
 * Uses the raw backendFixture (not test-base) on purpose — test-base's
 * auto-fixtures reset integration state through endpoints that require a
 * session once auth is enabled.
 */
test.describe.serial("opt-in authentication", () => {
  const ADMIN = { email: "admin@e2e.dev", password: "adminpass123", displayName: "Admin" };
  const MEMBER = { email: "member@e2e.dev", password: "memberpass123", displayName: "Member" };

  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });
  });

  test.afterAll(async ({ backend }) => {
    // Revert to the fixture's baseline env (auth off) for any later suites.
    await backend.restart();
  });

  test("boots into setup mode with no data exposed", async ({ browser, backend }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });

    const boot = await fetchBootState(context, backend.baseUrl);
    const auth = boot.initialState.auth as { mode: string; authenticated: boolean };
    expect(auth.mode).toBe("setup");
    expect(auth.authenticated).toBe(false);
    // No data loaders may run for anonymous visitors.
    expect(boot.initialState.workspaces).toBeUndefined();

    // Protected APIs are closed; allowlisted ones stay open.
    expect((await context.request.get(`${backend.baseUrl}/api/v1/workspaces`)).status()).toBe(401);
    expect((await context.request.get(`${backend.baseUrl}/health`)).ok()).toBeTruthy();
    expect((await context.request.get(`${backend.baseUrl}/api/v1/features`)).ok()).toBeTruthy();

    await context.close();
  });

  test("setup wizard promotes the first admin and signs them in", async ({ browser, backend }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });

    await setupAdmin(context, backend.baseUrl, ADMIN);

    // The session cookie from setup authenticates subsequent requests.
    const me = await (await context.request.get(`${backend.baseUrl}/api/v1/auth/me`)).json();
    expect(me.mode).toBe("enabled");
    expect(me.authenticated).toBe(true);
    expect(me.user.email).toBe(ADMIN.email);
    expect(me.user.role).toBe("admin");

    expect((await context.request.get(`${backend.baseUrl}/api/v1/workspaces`)).ok()).toBeTruthy();

    // Setup is single-shot.
    const again = await context.request.post(`${backend.baseUrl}/api/v1/auth/setup`, {
      data: { email: "second@e2e.dev", password: "password123" },
    });
    expect(again.status()).toBe(409);

    await context.close();
  });

  test("login rejects bad credentials and issues working sessions", async ({
    browser,
    backend,
  }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });

    const bad = await context.request.post(`${backend.baseUrl}/api/v1/auth/login`, {
      data: { email: ADMIN.email, password: "wrong-password!" },
    });
    expect(bad.status()).toBe(401);
    const ghost = await context.request.post(`${backend.baseUrl}/api/v1/auth/login`, {
      data: { email: "ghost@e2e.dev", password: "whatever-pass" },
    });
    expect(ghost.status()).toBe(401);

    await login(context, backend.baseUrl, ADMIN);
    expect((await context.request.get(`${backend.baseUrl}/api/v1/workspaces`)).ok()).toBeTruthy();

    // Logout revokes the session.
    await context.request.post(`${backend.baseUrl}/api/v1/auth/logout`);
    expect((await context.request.get(`${backend.baseUrl}/api/v1/workspaces`)).status()).toBe(401);

    await context.close();
  });

  test("two users are fully segregated over HTTP, boot payload, and WS", async ({
    browser,
    backend,
  }) => {
    test.setTimeout(90_000);
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(adminContext, backend.baseUrl, ADMIN);

    // Admin creates a private workspace.
    const created = await adminContext.request.post(`${backend.baseUrl}/api/v1/workspaces`, {
      data: { name: "Admin Secret Workspace" },
    });
    expect(created.ok(), await created.text()).toBeTruthy();
    const workspace = (await created.json()) as { id: string };

    // Member joins via invite.
    const inviteToken = await createInviteToken(adminContext, backend.baseUrl, {
      email: MEMBER.email,
      role: "member",
    });
    const memberContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await acceptInvite(memberContext, backend.baseUrl, inviteToken, MEMBER);

    // HTTP: the member cannot list or fetch the admin's workspace.
    const memberList = (await (
      await memberContext.request.get(`${backend.baseUrl}/api/v1/workspaces`)
    ).json()) as { workspaces: Array<{ id: string }> };
    expect(memberList.workspaces.map((w) => w.id)).not.toContain(workspace.id);
    expect(
      (
        await memberContext.request.get(`${backend.baseUrl}/api/v1/workspaces/${workspace.id}`)
      ).status(),
    ).toBeGreaterThanOrEqual(404);

    // Boot payload: member sees no admin data.
    const memberBoot = await fetchBootState(memberContext, backend.baseUrl);
    const memberWorkspaces = memberBoot.initialState.workspaces as
      | { items: Array<{ id: string }> }
      | undefined;
    if (memberWorkspaces) {
      expect(memberWorkspaces.items.map((w) => w.id)).not.toContain(workspace.id);
    }

    // Integration settings are also shielded: the member cannot read the
    // admin's workspace-scoped integration config by supplying the ID.
    for (const path of [
      `/api/v1/jira/config?workspace_id=${workspace.id}`,
      `/api/v1/github/status?workspace_id=${workspace.id}`,
      `/api/v1/gitlab/status?workspace_id=${workspace.id}`,
      `/api/v1/gitlab/workspaces/${workspace.id}/task-mrs`,
    ]) {
      const res = await memberContext.request.get(`${backend.baseUrl}${path}`);
      expect(res.status(), `integration route ${path} must be shielded`).toBe(404);
    }

    // Admin still sees it everywhere.
    const adminList = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/workspaces`)
    ).json()) as { workspaces: Array<{ id: string }> };
    expect(adminList.workspaces.map((w) => w.id)).toContain(workspace.id);

    // WS: member's socket must stay silent for admin workspace events.
    const memberPage = await memberContext.newPage();
    await memberPage.goto("/login");
    const wsUrl = backend.baseUrl.replace(/^http/, "ws") + "/ws";
    await memberPage.evaluate((url) => {
      const w = window as unknown as { __wsFrames: string[]; __ws?: WebSocket };
      w.__wsFrames = [];
      const socket = new WebSocket(url);
      socket.onmessage = (event) => w.__wsFrames.push(String(event.data));
      w.__ws = socket;
      return new Promise<void>((resolve, reject) => {
        socket.onopen = () => resolve();
        socket.onerror = () => reject(new Error("ws failed to open"));
      });
    }, wsUrl);

    // Admin mutates their workspace — generates workspace.updated traffic.
    const renamed = await adminContext.request.patch(
      `${backend.baseUrl}/api/v1/workspaces/${workspace.id}`,
      { data: { name: "Admin Secret Workspace v2" } },
    );
    expect(renamed.ok()).toBeTruthy();

    await memberPage.waitForTimeout(1500);
    const frames = await memberPage.evaluate(
      () => (window as unknown as { __wsFrames: string[] }).__wsFrames,
    );
    const leaked = frames.filter((frame) => frame.includes(workspace.id));
    expect(leaked, `member WS received admin workspace traffic: ${leaked[0] ?? ""}`).toHaveLength(
      0,
    );

    // Unauthenticated sockets are refused entirely.
    const anonContext = await browser.newContext({ baseURL: backend.frontendUrl });
    const anonPage = await anonContext.newPage();
    await anonPage.goto("/login");
    const anonRejected = await anonPage.evaluate((url) => {
      return new Promise<boolean>((resolve) => {
        const socket = new WebSocket(url);
        socket.onopen = () => resolve(false);
        socket.onclose = () => resolve(true);
        socket.onerror = () => resolve(true);
        setTimeout(() => resolve(false), 3000);
      });
    }, wsUrl);
    expect(anonRejected, "anonymous WS upgrade must be rejected").toBe(true);

    await anonContext.close();
    await memberContext.close();
    await adminContext.close();
  });

  test("personal access tokens authenticate programmatic clients", async ({ browser, backend }) => {
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(adminContext, backend.baseUrl, ADMIN);
    const pat = await mintPAT(adminContext, backend.baseUrl, "e2e-token");

    const bare = await browser.newContext();
    const withToken = await bare.request.get(`${backend.baseUrl}/api/v1/workspaces`, {
      headers: { Authorization: `Bearer ${pat}` },
    });
    expect(withToken.ok()).toBeTruthy();

    const forged = await bare.request.get(`${backend.baseUrl}/api/v1/workspaces`, {
      headers: { Authorization: "Bearer kandev_pat_deadbeef_forged" },
    });
    expect(forged.status()).toBe(401);

    await bare.close();
    await adminContext.close();
  });

  test("member cannot use admin surfaces", async ({ browser, backend }) => {
    const memberContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(memberContext, backend.baseUrl, MEMBER);

    expect((await memberContext.request.get(`${backend.baseUrl}/api/v1/users`)).status()).toBe(403);
    // Admin-only invite creation is also forbidden for members.
    expect(
      (
        await memberContext.request.post(`${backend.baseUrl}/api/v1/auth/invites`, {
          data: { role: "admin" },
        })
      ).status(),
    ).toBe(403);

    await memberContext.close();
  });

  test("login page renders for anonymous visitors", async ({ browser, backend }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    const page = await context.newPage();
    await page.goto("/");

    // The SPA must show the login surface instead of the app shell.
    await expect(page.getByTestId("login-email")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("login-password")).toBeVisible();

    // Sign in through the UI and land in the app.
    await page.getByTestId("login-email").fill(ADMIN.email);
    await page.getByTestId("login-password").fill(ADMIN.password);
    await page.getByTestId("login-submit").click();
    await page.waitForURL("**/");
    await expect(page.getByTestId("login-email")).toHaveCount(0, { timeout: 15_000 });

    await context.close();
  });
});
