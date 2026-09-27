import { expect } from "@playwright/test";
import type { BrowserContext } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";

/**
 * A workspace `viewer` holds only `workspace.read` (AC-COORDINATOR-COORDINATORS-003.2,
 * AC-COORDINATOR-COORDINATORS-004.6): the settings tab must render the list
 * with no `+ Add coordinator` and the coordinator's own page with every field
 * disabled and no Save or Delete, and the backend must refuse a reader's
 * create, edit or delete outright rather than relying on the UI to hide them.
 *
 * Runs in the `auth` project (backend restarted with auth required). Serial:
 * both tests share the admin, reader and coordinator seeded in beforeAll. A
 * per-file database keeps the single-shot auth setup from colliding with the
 * other auth specs sharing the worker; afterAll restarts to baseline.
 */
const GATING_ADMIN = {
  email: "coordinator-gating-admin@e2e.dev",
  password: "adminpass123",
  displayName: "Coordinator Gating Admin",
};

const GATING_READER = {
  email: "coordinator-gating-reader@e2e.dev",
  password: "readerpass123",
  displayName: "Coordinator Gating Reader",
};

const COORDINATOR_NAME = "Reader Gate Coordinator";

test.describe.serial("Coordinators settings tab reader gating", () => {
  let adminContext: BrowserContext;
  let readerContext: BrowserContext;
  let workspaceId = "";
  let coordinatorId = "";

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_FEATURES_COORDINATOR: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-coordinator.db"),
    });

    adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, GATING_ADMIN);
    await login(adminContext, backend.baseUrl, GATING_ADMIN);

    const token = await createInviteToken(adminContext, backend.baseUrl, {
      email: GATING_READER.email,
    });
    readerContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await acceptInvite(readerContext, backend.baseUrl, token, GATING_READER);
    await login(readerContext, backend.baseUrl, GATING_READER);

    const directory = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/users/directory`)
    ).json()) as { users: Array<{ id: string; display_name: string }> };
    const readerId = directory.users.find(
      (user) => user.display_name === GATING_READER.displayName,
    )?.id;
    expect(readerId, "reader must appear in the user directory").toBeTruthy();

    const createdWorkspace = await adminContext.request.post(
      `${backend.baseUrl}/api/v1/workspaces`,
      {
        data: { name: "Coordinator Reader Gating" },
      },
    );
    expect(createdWorkspace.ok(), await createdWorkspace.text()).toBeTruthy();
    workspaceId = (await createdWorkspace.json()).id;

    const addedMember = await adminContext.request.put(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/members/${readerId}`,
      { data: { role: "viewer" } },
    );
    expect(addedMember.ok(), await addedMember.text()).toBeTruthy();

    // Seed against the default mock agent profile and worktree executor
    // profile, which are always present so no separate profile setup is
    // needed for this gating-only spec.
    const agents = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/agents`)
    ).json()) as { agents: Array<{ profiles: Array<{ id: string; cli_passthrough: boolean }> }> };
    const agentProfileId = agents.agents
      .flatMap((agent) => agent.profiles)
      .find((profile) => !profile.cli_passthrough)?.id;
    expect(agentProfileId, "a non-passthrough agent profile must exist").toBeTruthy();

    const executors = (await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/executors`)
    ).json()) as { executors: Array<{ type: string; profiles?: Array<{ id: string }> }> };
    const executorProfileId = executors.executors.find((executor) => executor.type === "worktree")
      ?.profiles?.[0]?.id;
    expect(executorProfileId, "the worktree executor profile must exist").toBeTruthy();

    const createdCoordinator = await adminContext.request.post(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/coordinators`,
      {
        data: {
          name: COORDINATOR_NAME,
          agent_profile_id: agentProfileId,
          executor_profile_id: executorProfileId,
          context: "",
        },
      },
    );
    expect(createdCoordinator.ok(), await createdCoordinator.text()).toBeTruthy();
    coordinatorId = (await createdCoordinator.json()).id;
  });

  test.afterAll(async ({ backend }) => {
    await adminContext?.close();
    await readerContext?.close();
    await backend.restart();
  });

  test("a reader sees the list with no Add and the editor disabled with no Save or Delete (AC-004.6)", async () => {
    const page = await readerContext.newPage();

    await page.goto(`/settings/workspaces/${workspaceId}/coordinators`);
    await expect(page.getByTestId("coordinators-list-page")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId("add-coordinator-button")).toHaveCount(0);
    await expect(page.getByTestId(`coordinator-card-${coordinatorId}`)).toContainText(
      COORDINATOR_NAME,
    );

    await page.goto(`/settings/workspaces/${workspaceId}/coordinators/${coordinatorId}`);
    await expect(page.getByTestId("coordinator-editor-page")).toBeVisible({ timeout: 15_000 });
    await expect(page.getByLabel("Name")).toBeDisabled();
    await expect(page.getByLabel("Context")).toBeDisabled();
    await expect(page.getByTestId("delete-coordinator-button")).toHaveCount(0);
    await expect(page.getByTestId("settings-floating-save")).toHaveCount(0);

    await page.close();
  });

  test("a reader's create, edit and delete requests are refused with 403 and change nothing (AC-003.2)", async ({
    backend,
  }) => {
    const api = readerContext.request;
    const coordinators = `${backend.baseUrl}/api/v1/workspaces/${workspaceId}/coordinators`;

    const created = await api.post(coordinators, {
      data: {
        name: "Reader Attempt",
        agent_profile_id: "does-not-matter",
        executor_profile_id: "does-not-matter",
        context: "",
      },
    });
    expect(created.status(), await created.text()).toBe(403);

    const patched = await api.patch(`${coordinators}/${coordinatorId}`, {
      data: { name: "Changed by reader" },
    });
    expect(patched.status(), await patched.text()).toBe(403);

    const deleted = await api.delete(`${coordinators}/${coordinatorId}`);
    expect(deleted.status(), await deleted.text()).toBe(403);

    const list = await api.get(coordinators);
    expect(list.status(), await list.text()).toBe(200);
    const body = (await list.json()) as { coordinators: Array<{ id: string; name: string }> };
    expect(body.coordinators).toContainEqual(
      expect.objectContaining({ id: coordinatorId, name: COORDINATOR_NAME }),
    );
  });
});
