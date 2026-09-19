import { expect, type APIResponse } from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, setupAdmin } from "../../helpers/auth";

type ShareResponse = {
  id: string;
  revoked_at?: string;
};

type SnapshotResponse = {
  messages?: Array<{ blocks?: Array<{ text?: string }> }>;
};

async function expectJSON<T>(response: APIResponse, status: number): Promise<T> {
  const body = await response.text();
  expect(response.status(), body).toBe(status);
  return JSON.parse(body) as T;
}

async function expectNotFound(response: APIResponse, forbidden: string[]): Promise<void> {
  const body = await response.text();
  expect(response.status(), body).toBe(404);
  for (const value of forbidden) {
    expect(body).not.toContain(value);
  }
}

test.describe.serial("share authorization", () => {
  const ADMIN = { email: "share-admin@e2e.dev", password: "adminpass123", displayName: "Admin" };
  const MEMBER_A = {
    email: "share-attacker@e2e.dev",
    password: "attackerpass123",
    displayName: "Member A",
  };
  const MEMBER_B = {
    email: "share-owner@e2e.dev",
    password: "ownerpass123",
    displayName: "Member B",
  };

  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_AUTH: "true" });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("keeps a member's share routes private from another user", async ({ browser, backend }) => {
    test.setTimeout(90_000);
    const adminContext = await browser.newContext({ baseURL: backend.frontendUrl });
    const attackerContext = await browser.newContext({ baseURL: backend.frontendUrl });
    const ownerContext = await browser.newContext({ baseURL: backend.frontendUrl });

    try {
      await setupAdmin(adminContext, backend.baseUrl, ADMIN);
      const attackerInviteToken = await createInviteToken(adminContext, backend.baseUrl, {
        email: MEMBER_A.email,
        role: "member",
      });
      const ownerInviteToken = await createInviteToken(adminContext, backend.baseUrl, {
        email: MEMBER_B.email,
        role: "member",
      });
      await acceptInvite(attackerContext, backend.baseUrl, attackerInviteToken, MEMBER_A);
      await acceptInvite(ownerContext, backend.baseUrl, ownerInviteToken, MEMBER_B);

      const workspaceResponse = await ownerContext.request.post(
        `${backend.baseUrl}/api/v1/workspaces`,
        { data: { name: "Private Share Workspace" } },
      );
      const workspace = await expectJSON<{ id: string }>(workspaceResponse, 200);

      const githubResponse = await ownerContext.request.put(
        `${backend.baseUrl}/api/v1/github/mock/workspace-connections/${workspace.id}`,
        {
          data: {
            source: "pat",
            status: "active",
            login: "share-owner-mock",
          },
        },
      );
      await expectJSON(githubResponse, 200);

      const taskResponse = await ownerContext.request.post(
        `${backend.baseUrl}/api/v1/_test/tasks`,
        {
          data: {
            workspace_id: workspace.id,
            title: "Private share task",
            state: "IN_PROGRESS",
          },
        },
      );
      const task = await expectJSON<{ task_id: string }>(taskResponse, 200);

      const sessionResponse = await ownerContext.request.post(
        `${backend.baseUrl}/api/v1/_test/task-sessions`,
        {
          data: {
            task_id: task.task_id,
            state: "COMPLETED",
            completed_at: new Date().toISOString(),
          },
        },
      );
      const session = await expectJSON<{ session_id: string }>(sessionResponse, 200);

      const transcriptMarker = "B_PRIVATE_SHARE_TRANSCRIPT_MARKER";
      const messageResponse = await ownerContext.request.post(
        `${backend.baseUrl}/api/v1/_test/messages`,
        {
          data: {
            session_id: session.session_id,
            type: "message",
            content: transcriptMarker,
          },
        },
      );
      await expectJSON(messageResponse, 200);

      const sharePath = `/api/v1/tasks/${task.task_id}/sessions/${session.session_id}/shares`;
      const ownerPreview = await ownerContext.request.post(
        `${backend.baseUrl}${sharePath}?dry_run=true`,
      );
      const preview = await expectJSON<SnapshotResponse>(ownerPreview, 200);
      expect(JSON.stringify(preview)).toContain(transcriptMarker);

      const foreignValues = [task.task_id, session.session_id, transcriptMarker];
      const attackerPreview = await attackerContext.request.post(
        `${backend.baseUrl}${sharePath}?dry_run=true`,
      );
      await expectNotFound(attackerPreview, foreignValues);

      const attackerPublish = await attackerContext.request.post(`${backend.baseUrl}${sharePath}`);
      await expectNotFound(attackerPublish, foreignValues);

      const ownerSharesBeforePublish = await ownerContext.request.get(
        `${backend.baseUrl}${sharePath}`,
      );
      const emptyShares = await expectJSON<{ shares: ShareResponse[] }>(
        ownerSharesBeforePublish,
        200,
      );
      expect(emptyShares.shares).toHaveLength(0);

      const ownerPublish = await ownerContext.request.post(`${backend.baseUrl}${sharePath}`);
      const share = await expectJSON<ShareResponse>(ownerPublish, 201);
      expect(share.id).toBeTruthy();

      const attackerList = await attackerContext.request.get(`${backend.baseUrl}${sharePath}`);
      await expectNotFound(attackerList, [task.task_id, session.session_id, share.id]);

      const attackerRevoke = await attackerContext.request.delete(
        `${backend.baseUrl}/api/v1/shares/${share.id}`,
      );
      await expectNotFound(attackerRevoke, [share.id]);

      const ownerSharesAfterDeniedRevoke = await ownerContext.request.get(
        `${backend.baseUrl}${sharePath}`,
      );
      const activeShares = await expectJSON<{ shares: ShareResponse[] }>(
        ownerSharesAfterDeniedRevoke,
        200,
      );
      expect(activeShares.shares).toEqual([expect.objectContaining({ id: share.id })]);
      expect(activeShares.shares[0]?.revoked_at).toBeUndefined();

      const otherTaskResponse = await ownerContext.request.post(
        `${backend.baseUrl}/api/v1/_test/tasks`,
        {
          data: {
            workspace_id: workspace.id,
            title: "Mismatched share task",
            state: "IN_PROGRESS",
          },
        },
      );
      const otherTask = await expectJSON<{ task_id: string }>(otherTaskResponse, 200);
      const mismatchedPath = `/api/v1/tasks/${otherTask.task_id}/sessions/${session.session_id}/shares`;

      const mismatchedPreview = await attackerContext.request.post(
        `${backend.baseUrl}${mismatchedPath}?dry_run=true`,
      );
      await expectNotFound(mismatchedPreview, foreignValues);

      const mismatchedList = await attackerContext.request.get(
        `${backend.baseUrl}${mismatchedPath}`,
      );
      await expectNotFound(mismatchedList, [share.id, transcriptMarker]);

      const ownerRevoke = await ownerContext.request.delete(
        `${backend.baseUrl}/api/v1/shares/${share.id}`,
      );
      const revokeBody = await ownerRevoke.text();
      expect(ownerRevoke.status(), revokeBody).toBe(204);

      const revokedSharesResponse = await ownerContext.request.get(
        `${backend.baseUrl}${sharePath}`,
      );
      const revokedShares = await expectJSON<{ shares: ShareResponse[] }>(
        revokedSharesResponse,
        200,
      );
      expect(revokedShares.shares).toEqual([
        expect.objectContaining({ id: share.id, revoked_at: expect.any(String) }),
      ]);
    } finally {
      await ownerContext.close();
      await attackerContext.close();
      await adminContext.close();
    }
  });
});
