import { expect, type Page, type BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";

const ADMIN = {
  email: "executor-admin@e2e.dev",
  password: "adminpass123",
  displayName: "Executor Admin",
};
const ALICE = {
  email: "executor-alice@e2e.dev",
  password: "alicepass123",
  displayName: "Alice Owner",
};
const BOB = { email: "executor-bob@e2e.dev", password: "bobpass123", displayName: "Bob Member" };
const PRIVATE_NAME = "Alice private runner";
const PRIVATE_SCRIPT = "echo alice-private-executor-script";

async function chooseOwner(page: Page, mobile: boolean, name: string, touchInput = mobile) {
  if (mobile) {
    const trigger = page.getByRole("button", { name: "Profile access", exact: true });
    await expect(trigger).toBeVisible();
    expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    if (touchInput) await trigger.tap();
    else await trigger.click();
    const drawer = page.getByRole("dialog");
    await expect(drawer.getByRole("heading", { name: "Profile access" })).toBeVisible();
    await expect(drawer.getByRole("option", { name, exact: true })).toBeInViewport();
    if (process.env.CAPTURE_PR_ASSETS) {
      await drawer.evaluate(async (element) => {
        await Promise.all(
          element.getAnimations({ subtree: true }).map((animation) => animation.finished),
        );
      });
      await page.screenshot({ path: ".pr-assets/executor-ownership-mobile.png" });
    }
    const option = drawer.getByRole("option", { name, exact: true });
    if (touchInput) await option.tap();
    else await option.click();
    await expect(drawer).toBeHidden();
  } else {
    await page.getByRole("combobox", { name: "Profile access", exact: true }).click();
    await page.getByRole("option", { name, exact: true }).click();
  }
}

export function registerExecutorOwnershipTests(mobile: boolean) {
  let admin: BrowserContext;
  let alice: BrowserContext;
  let bob: BrowserContext;
  let aliceId: string;
  let bobId: string;
  let executorId: string;
  let profileId: string;
  let baseUrl: string;

  test.describe.serial("private executor ownership", () => {
    test.beforeAll(async ({ backend, browser }) => {
      await backend.restart({
        KANDEV_FEATURES_AUTH: "true",
        KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "executor-ownership-auth.db"),
      });
      baseUrl = backend.baseUrl;
      admin = await browser.newContext({ baseURL: backend.frontendUrl });
      alice = await browser.newContext({ baseURL: backend.frontendUrl });
      bob = await browser.newContext({ baseURL: backend.frontendUrl });
      await setupAdmin(admin, baseUrl, ADMIN);
      await login(admin, baseUrl, ADMIN);
      await acceptInvite(alice, baseUrl, await createInviteToken(admin, baseUrl), ALICE);
      await acceptInvite(bob, baseUrl, await createInviteToken(admin, baseUrl), BOB);
      aliceId = (await (await alice.request.get(`${baseUrl}/api/v1/auth/me`)).json()).user.id;
      bobId = (await (await bob.request.get(`${baseUrl}/api/v1/auth/me`)).json()).user.id;
      const catalogue = await (await admin.request.get(`${baseUrl}/api/v1/executors`)).json();
      executorId = catalogue.executors.find(
        (executor: { type: string }) => executor.type === "local",
      ).id;
      const created = await alice.request.post(
        `${baseUrl}/api/v1/executors/${executorId}/profiles`,
        { data: { name: PRIVATE_NAME, owner_user_id: aliceId, prepare_script: PRIVATE_SCRIPT } },
      );
      expect(created.ok(), await created.text()).toBe(true);
      profileId = (await created.json()).id;
    });

    test.afterAll(async ({ backend }) => {
      await Promise.all([admin?.close(), alice?.close(), bob?.close()]);
      await backend.restart();
    });

    test("filters API and boot data and rejects foreign mutations", async () => {
      const url = `${baseUrl}/api/v1/executors/${executorId}/profiles/${profileId}`;
      for (const response of [
        await bob.request.get(url),
        await bob.request.patch(url, { data: { name: "stolen" } }),
        await bob.request.delete(url),
      ])
        expect(response.status()).toBe(404);
      const catalogue = await bob.request.get(`${baseUrl}/api/v1/executors`);
      expect(await catalogue.text()).not.toContain(PRIVATE_SCRIPT);
      const boot = await bob.request.get(`${baseUrl}/api/v1/app-state?path=/settings/executors`);
      expect(boot.ok()).toBe(true);
      expect(await boot.text()).not.toContain(PRIVATE_SCRIPT);
      expect((await alice.request.get(url)).ok()).toBe(true);
      expect((await admin.request.get(url)).ok()).toBe(true);
    });

    test("keeps unsaved profile and ownership drafts across an unrelated catalogue invalidation", async ({
      context,
      page,
    }) => {
      await login(context, baseUrl, ALICE);
      await page.goto(`${baseUrl}/settings/executors/${profileId}`);
      await page.getByLabel("Name", { exact: true }).fill("Unsaved personal runner");
      await chooseOwner(page, mobile, "Shared");
      let release!: () => void;
      let requested!: () => void;
      const pending = new Promise<void>((resolve) => {
        release = resolve;
      });
      const received = new Promise<void>((resolve) => {
        requested = resolve;
      });
      await page.route("**/api/v1/executors", async (route) => {
        requested();
        await pending;
        await route.continue();
      });
      try {
        const created = await admin.request.post(
          `${baseUrl}/api/v1/executors/${executorId}/profiles`,
          { data: { name: "Unrelated shared runner" } },
        );
        expect(created.ok(), await created.text()).toBe(true);
        await received;
        await expect(page.getByLabel("Name", { exact: true })).toBeHidden();
      } finally {
        release();
      }
      await expect(page.getByLabel("Name", { exact: true })).toHaveValue("Unsaved personal runner");
      await expect(
        page.getByRole(mobile ? "button" : "combobox", { name: "Profile access", exact: true }),
      ).toHaveText("Shared");
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Reset", exact: true })
        .click();
      await page.unroute("**/api/v1/executors");
    });

    test("owner shares the profile and an ownership transfer evicts an open editor", async ({
      context,
      page,
    }) => {
      await login(context, baseUrl, ALICE);
      await page.goto(`${baseUrl}/settings/executors/${profileId}`);
      await chooseOwner(page, mobile, "Shared");
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Reset", exact: true })
        .click();
      await expect(
        page.getByRole(mobile ? "button" : "combobox", { name: "Profile access", exact: true }),
      ).toHaveText("Only me");
      await chooseOwner(page, mobile, "Shared");
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes", exact: true })
        .click();
      await expect
        .poll(async () =>
          (
            await bob.request.get(`${baseUrl}/api/v1/executors/${executorId}/profiles/${profileId}`)
          ).status(),
        )
        .toBe(200);
      const transferred = await admin.request.patch(
        `${baseUrl}/api/v1/executors/${executorId}/profiles/${profileId}`,
        { data: { owner_user_id: bobId } },
      );
      expect(transferred.ok(), await transferred.text()).toBe(true);
      await expect(page.getByRole("heading", { name: PRIVATE_NAME, exact: true })).toHaveCount(0);
      await expect(page.getByRole("combobox", { name: "Profile access", exact: true })).toHaveCount(
        0,
      );
      await expect(page.getByRole("button", { name: "Profile access", exact: true })).toHaveCount(
        0,
      );
      expect(
        (
          await alice.request.get(`${baseUrl}/api/v1/executors/${executorId}/profiles/${profileId}`)
        ).status(),
      ).toBe(404);
      if (mobile)
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        ).toBe(true);
    });

    test("creates a personal profile and lets an administrator reassign it", async ({
      context,
      page,
    }) => {
      await login(context, baseUrl, ALICE);
      await page.goto(`${baseUrl}/settings/executors/new/local`);
      await page.getByLabel("Name", { exact: true }).fill("Owner creation UI");
      await chooseOwner(page, mobile, "Only me");
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes", exact: true })
        .click();
      await expect(page).toHaveURL(/\/settings\/executors\/[^/]+$/);
      let catalogue = await (await alice.request.get(`${baseUrl}/api/v1/executors`)).json();
      const profile = catalogue.executors
        .flatMap(
          (executor: { profiles?: { id: string; name: string; owner_user_id: string }[] }) =>
            executor.profiles ?? [],
        )
        .find((entry: { name: string }) => entry.name === "Owner creation UI");
      expect(profile?.owner_user_id).toBe(aliceId);
      await login(context, baseUrl, ADMIN);
      await page.reload();
      await chooseOwner(page, mobile, "Bob Member");
      await page
        .getByTestId("settings-floating-save")
        .getByRole("button", { name: "Save changes", exact: true })
        .click();
      await expect
        .poll(async () => {
          catalogue = await (await bob.request.get(`${baseUrl}/api/v1/executors`)).json();
          return catalogue.executors
            .flatMap(
              (executor: { profiles?: { id: string; owner_user_id: string }[] }) =>
                executor.profiles ?? [],
            )
            .find((entry: { id: string }) => entry.id === profile.id)?.owner_user_id;
        })
        .toBe(bobId);
      if (mobile)
        expect(
          await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
        ).toBe(true);
    });

    test("launches for the owner and rejects another shared-board collaborator", async () => {
      test.setTimeout(90_000);
      const workspaceResponse = await bob.request.post(`${baseUrl}/api/v1/workspaces`, {
        data: { name: "Private executor shared board" },
      });
      expect(workspaceResponse.ok(), await workspaceResponse.text()).toBe(true);
      const workspace = await workspaceResponse.json();
      const share = await bob.request.put(
        `${baseUrl}/api/v1/workspaces/${workspace.id}/members/${aliceId}`,
        { data: { role: "collaborator" } },
      );
      expect(share.ok(), await share.text()).toBe(true);
      const templates = await (
        await bob.request.get(`${baseUrl}/api/v1/workflow/templates`)
      ).json();
      const workflowResponse = await bob.request.post(`${baseUrl}/api/v1/workflows`, {
        data: {
          workspace_id: workspace.id,
          name: "Private runner workflow",
          workflow_template_id: templates.templates[0].id,
        },
      });
      expect(workflowResponse.ok(), await workflowResponse.text()).toBe(true);
      const workflow = await workflowResponse.json();
      const agents = await (await bob.request.get(`${baseUrl}/api/v1/agents`)).json();
      const agent = agents.agents.find(
        (entry: { id: string; profiles: { id: string }[] }) =>
          entry.id !== "dynamic" && entry.profiles?.length,
      );
      expect(agent).toBeDefined();
      const taskBody = {
        workspace_id: workspace.id,
        workflow_id: workflow.id,
        title: "Owner can launch",
        description: "Say hello",
        agent_profile_id: agent.profiles[0].id,
        executor_profile_id: profileId,
        start_agent: true,
      };
      const created = await bob.request.post(`${baseUrl}/api/v1/tasks`, { data: taskBody });
      expect(created.ok(), await created.text()).toBe(true);
      const task = await created.json();
      expect(task.session_id).toBeTruthy();
      await expect
        .poll(
          async () => {
            const response = await bob.request.get(
              `${baseUrl}/api/v1/task-sessions/${task.session_id}`,
            );
            if (!response.ok()) return response.status();
            const { session } = await response.json();
            expect(session.executor_profile_id).toBe(profileId);
            return session.state;
          },
          { timeout: 45_000 },
        )
        .toBe("WAITING_FOR_INPUT");
      expect((await alice.request.get(`${baseUrl}/api/v1/tasks/${task.id}`)).ok()).toBe(true);
      const denied = await alice.request.post(`${baseUrl}/api/v1/tasks`, {
        data: { ...taskBody, title: "Denied compute borrowing" },
      });
      expect(denied.ok(), await denied.text()).toBe(false);
      expect(denied.status()).toBeLessThan(500);
      const tasks = await (
        await bob.request.get(`${baseUrl}/api/v1/workspaces/${workspace.id}/tasks`)
      ).json();
      expect(tasks.tasks).toHaveLength(1);
    });

    if (!mobile)
      test("keeps a draft across the phone breakpoint with a fine pointer", async ({
        context,
        page,
      }) => {
        await login(context, baseUrl, ALICE);
        const created = await alice.request.post(
          `${baseUrl}/api/v1/executors/${executorId}/profiles`,
          { data: { name: "Breakpoint ownership", owner_user_id: aliceId } },
        );
        expect(created.ok(), await created.text()).toBe(true);
        const profile = await created.json();
        await page.goto(`${baseUrl}/settings/executors/${profile.id}`);
        await page.setViewportSize({ width: 767, height: 900 });
        await chooseOwner(page, true, "Shared", false);
        await page.setViewportSize({ width: 768, height: 900 });
        const selector = page.getByRole("combobox", { name: "Profile access", exact: true });
        await expect(selector).toHaveText("Shared");
        expect((await selector.boundingBox())!.height).toBeLessThan(44);
        await page
          .getByTestId("settings-floating-save")
          .getByRole("button", { name: "Reset", exact: true })
          .click();
        await expect(selector).toHaveText("Only me");
      });
  });
}
