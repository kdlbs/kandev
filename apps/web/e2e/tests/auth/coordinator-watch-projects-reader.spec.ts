import { expect } from "@playwright/test";
import type { BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";

/**
 * A workspace `viewer` sees the Projects controls of Watches disabled with no
 * Save, and the backend refuses the reader's write of a projects scope
 * (REQ-COORDINATOR-PERMISSIONS-005). Runs in the `auth` project with phase 3.1
 * on; afterAll restarts to baseline.
 */
const ADMIN = {
  email: "coordinator-projects-admin@e2e.dev",
  password: "adminpass123",
  displayName: "Coordinator Projects Admin",
};
const READER = {
  email: "coordinator-projects-reader@e2e.dev",
  password: "readerpass123",
  displayName: "Coordinator Projects Reader",
};

test.describe.serial("Coordinator watch projects reader gating", () => {
  let adminContext: BrowserContext;
  let readerContext: BrowserContext;
  let workspaceId = "";
  let coordinatorId = "";

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_FEATURES_COORDINATOR_PHASE31: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-coordinator-projects.db"),
    });
    adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, ADMIN);
    await login(adminContext, backend.baseUrl, ADMIN);

    const token = await createInviteToken(adminContext, backend.baseUrl, { email: READER.email });
    readerContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await acceptInvite(readerContext, backend.baseUrl, token, READER);
    await login(readerContext, backend.baseUrl, READER);

    const directory = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/users/directory`)
    ).json()) as { users: Array<{ id: string; display_name: string }> };
    const readerId = directory.users.find((u) => u.display_name === READER.displayName)?.id;
    expect(readerId, "reader must appear in the user directory").toBeTruthy();

    const created = await adminContext.request.post(`${backend.baseUrl}/api/v1/workspaces`, {
      data: { name: "Coordinator Projects Gating" },
    });
    expect(created.ok(), await created.text()).toBeTruthy();
    workspaceId = (await created.json()).id;
    const member = await adminContext.request.put(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/members/${readerId}`,
      { data: { role: "viewer" } },
    );
    expect(member.ok(), await member.text()).toBeTruthy();

    const agents = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/agents`)
    ).json()) as { agents: Array<{ profiles: Array<{ id: string; cli_passthrough: boolean }> }> };
    const agentProfileId = agents.agents
      .flatMap((agent) => agent.profiles)
      .find((profile) => !profile.cli_passthrough)?.id;
    const executors = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/executors`)
    ).json()) as { executors: Array<{ type: string; profiles?: Array<{ id: string }> }> };
    const executorProfileId = executors.executors.find((e) => e.type === "worktree")?.profiles?.[0]
      ?.id;
    const coordinator = await adminContext.request.post(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/coordinators`,
      {
        data: {
          name: "Projects Reader Gate",
          agent_profile_id: agentProfileId,
          executor_profile_id: executorProfileId,
          context: "",
        },
      },
    );
    expect(coordinator.ok(), await coordinator.text()).toBeTruthy();
    coordinatorId = (await coordinator.json()).id;
  });

  test.afterAll(async ({ backend }) => {
    await adminContext?.close();
    await readerContext?.close();
    await backend.restart();
  });

  test("a reader sees the Projects switch disabled with no Save, and a projects write gets 403", async ({
    backend,
  }) => {
    const page = await readerContext.newPage();
    await page.goto(
      `/settings/workspaces/${workspaceId}/coordinators/${coordinatorId}?section=watches`,
    );
    await expect(page.getByTestId("watches-projects")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("watches-projects-all")).toBeDisabled();
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);
    await page.close();

    const put = await readerContext.request.put(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/coordinators/${coordinatorId}/settings`,
      {
        data: {
          projects: { scope: "selected", entries: [], include_no_repository: true },
        },
      },
    );
    expect(put.status(), await put.text()).toBe(403);
  });
});
