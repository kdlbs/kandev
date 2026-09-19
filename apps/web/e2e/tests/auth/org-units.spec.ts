import { expect } from "@playwright/test";
import type { BrowserContext } from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";

/**
 * Reach through the organization unit tree.
 *
 * The unit tests prove the resolver's table and the handler's refusals. This
 * proves the outcome the tree exists for: someone reaches a board because of
 * where it sits, with nobody having named them on it, and stops reaching it
 * when it moves. A resolver nothing calls protects nothing, and a tree nothing
 * places workspaces into shares nothing.
 *
 * Runs in the `auth` project: reach only means something once identities do.
 */
test.describe.serial("organization units", () => {
  const ADMIN = { email: "ana@e2e.dev", password: "adminpass123", displayName: "Ana" };
  const MEMBER = { email: "bruno@e2e.dev", password: "memberpass123", displayName: "Bruno" };

  let adminContext: BrowserContext;
  let memberContext: BrowserContext;
  let rootId = "";
  let deptId = "";
  let teamId = "";
  let workspaceId = "";
  let memberId = "";

  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });

    adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(adminContext, backend.baseUrl, ADMIN);
    await login(adminContext, backend.baseUrl, ADMIN);

    const token = await createInviteToken(adminContext, backend.baseUrl, { email: MEMBER.email });
    memberContext = await browser.newContext({ baseURL: backend.frontendUrl });
    await acceptInvite(memberContext, backend.baseUrl, token, MEMBER);
    await login(memberContext, backend.baseUrl, MEMBER);

    const directory = await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/users/directory`)
    ).json();
    memberId = directory.users.find((u: { display_name: string }) =>
      u.display_name.includes("Bruno"),
    ).id;
  });

  test.afterAll(async ({ backend }) => {
    await adminContext?.close();
    await memberContext?.close();
    await backend.restart();
  });

  test("builds a department and a team beneath the organization", async ({ backend }) => {
    const tree = await (await adminContext.request.get(`${backend.baseUrl}/api/v1/units`)).json();
    rootId = tree.units.find((u: { kind: string }) => u.kind === "root").id;

    const dept = await adminContext.request.post(`${backend.baseUrl}/api/v1/units`, {
      data: { parent_id: rootId, name: "Platform" },
    });
    expect(dept.status(), await dept.text()).toBe(201);
    deptId = (await dept.json()).id;

    const team = await adminContext.request.post(`${backend.baseUrl}/api/v1/units`, {
      data: { parent_id: deptId, name: "Runtime" },
    });
    expect(team.status(), await team.text()).toBe(201);
    teamId = (await team.json()).id;
  });

  test("a member reaches a board through the unit above it, uninvited", async ({ backend }) => {
    const created = await adminContext.request.post(`${backend.baseUrl}/api/v1/workspaces`, {
      data: { name: "Platform Team" },
    });
    expect(created.ok(), await created.text()).toBeTruthy();
    workspaceId = (await created.json()).id;

    // Created by the admin, so it starts in her personal unit: nobody else.
    const before = await memberContext.request.get(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
    );
    expect(before.status()).toBe(404);

    // Put the board in the team and the person in the department above it.
    const moved = await adminContext.request.patch(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
      { data: { unit_id: teamId } },
    );
    expect(moved.ok(), await moved.text()).toBeTruthy();

    const added = await adminContext.request.put(
      `${backend.baseUrl}/api/v1/units/${deptId}/members/${memberId}`,
      { data: { role: "collaborator" } },
    );
    expect(added.ok(), await added.text()).toBeTruthy();

    const after = await memberContext.request.get(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
    );
    expect(
      after.status(),
      "a member of the department must reach a board in its team with no invitation",
    ).toBe(200);
  });

  test("moving the board out of the tree withdraws that reach", async ({ backend }) => {
    const personal = await (
      await adminContext.request.get(`${backend.baseUrl}/api/v1/units`)
    ).json();
    const personalId = personal.units.find((u: { kind: string }) => u.kind === "personal").id;

    const moved = await adminContext.request.patch(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
      { data: { unit_id: personalId } },
    );
    expect(moved.ok(), await moved.text()).toBeTruthy();

    const after = await memberContext.request.get(
      `${backend.baseUrl}/api/v1/workspaces/${workspaceId}`,
    );
    expect(after.status(), "a board in someone's personal unit reaches nobody else").toBe(404);
  });

  test("a member cannot rearrange the organization", async ({ backend }) => {
    const created = await memberContext.request.post(`${backend.baseUrl}/api/v1/units`, {
      data: { parent_id: rootId, name: "Shadow department" },
    });
    expect(created.status(), "unit.manage is an administrator scope").toBe(403);

    // Reading the tree is allowed: a member has to see why they reach what
    // they reach.
    const read = await memberContext.request.get(`${backend.baseUrl}/api/v1/units`);
    expect(read.status()).toBe(200);
  });

  test("the tree renders in settings", async ({ backend }) => {
    const page = await adminContext.newPage();
    await page.addInitScript(() =>
      window.localStorage.setItem("kandev.onboarding.completed", "true"),
    );
    await page.goto(`${backend.frontendUrl}/settings/units`);

    const rows = page.getByTestId("unit-row");
    await expect(rows.first()).toBeVisible();
    // Scoped to the rows: "Platform" also appears as a workspace name, and a
    // bare text match would pass for the wrong reason.
    await expect(rows.filter({ hasText: "Platform" }).first()).toBeVisible();
    await expect(rows.filter({ hasText: "Runtime" }).first()).toBeVisible();
    await page.close();
  });
});
