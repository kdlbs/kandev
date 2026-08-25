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
  const MEMBER = {
    email: "share-member@e2e.dev",
    password: "memberpass123",
    displayName: "Member",
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
    const memberContext = await browser.newContext({ baseURL: backend.frontendUrl });

    try {
      await setupAdmin(adminContext, backend.baseUrl, ADMIN);
      const inviteToken = await createInviteToken(adminContext, backend.baseUrl, {
        email: MEMBER.email,
        role: "member",
      });
      await acceptInvite(memberContext, backend.baseUrl, inviteToken, MEMBER);

      const workspaceResponse = await memberContext.request.post(
        `${backend.baseUrl}/api/v1/workspaces`,
        { data: { name: "Private Share Workspace" } },
      );
      const workspace = await expectJSON<{ id: string }>(workspaceResponse, 200);

      const githubResponse = await memberContext.request.put(
        `${backend.baseUrl}/api/v1/github/mock/workspace-connections/${workspace.id}`,
        {
          data: {
            source: "pat",
            status: "active",
            login: "share-member-mock",
          },
        },
      );
      await expectJSON(githubResponse, 200);

      const taskResponse = await memberContext.request.post(
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

      const sessionResponse = await memberContext.request.post(
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
      const messageResponse = await memberContext.request.post(
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
      const memberPreview = await memberContext.request.post(
        `${backend.baseUrl}${sharePath}?dry_run=true`,
      );
      const preview = await expectJSON<SnapshotResponse>(memberPreview, 200);
      expect(JSON.stringify(preview)).toContain(transcriptMarker);

      const foreignValues = [task.task_id, session.session_id, transcriptMarker];
      const adminPreview = await adminContext.request.post(
        `${backend.baseUrl}${sharePath}?dry_run=true`,
      );
      await expectNotFound(adminPreview, foreignValues);

      const adminPublish = await adminContext.request.post(`${backend.baseUrl}${sharePath}`);
      await expectNotFound(adminPublish, foreignValues);

      const memberSharesBeforePublish = await memberContext.request.get(
        `${backend.baseUrl}${sharePath}`,
      );
      const emptyShares = await expectJSON<{ shares: ShareResponse[] }>(
        memberSharesBeforePublish,
        200,
      );
      expect(emptyShares.shares).toHaveLength(0);

      const memberPublish = await memberContext.request.post(`${backend.baseUrl}${sharePath}`);
      const share = await expectJSON<ShareResponse>(memberPublish, 201);
      expect(share.id).toBeTruthy();

      const adminList = await adminContext.request.get(`${backend.baseUrl}${sharePath}`);
      await expectNotFound(adminList, [task.task_id, session.session_id, share.id]);

      const adminRevoke = await adminContext.request.delete(
        `${backend.baseUrl}/api/v1/shares/${share.id}`,
      );
      await expectNotFound(adminRevoke, [share.id]);

      const memberSharesAfterDeniedRevoke = await memberContext.request.get(
        `${backend.baseUrl}${sharePath}`,
      );
      const activeShares = await expectJSON<{ shares: ShareResponse[] }>(
        memberSharesAfterDeniedRevoke,
        200,
      );
      expect(activeShares.shares).toEqual([expect.objectContaining({ id: share.id })]);
      expect(activeShares.shares[0]?.revoked_at).toBeUndefined();

      const otherTaskResponse = await memberContext.request.post(
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

      const mismatchedPreview = await memberContext.request.post(
        `${backend.baseUrl}${mismatchedPath}?dry_run=true`,
      );
      await expectNotFound(mismatchedPreview, foreignValues);

      const mismatchedList = await memberContext.request.get(`${backend.baseUrl}${mismatchedPath}`);
      await expectNotFound(mismatchedList, [share.id, transcriptMarker]);

      const memberRevoke = await memberContext.request.delete(
        `${backend.baseUrl}/api/v1/shares/${share.id}`,
      );
      const revokeBody = await memberRevoke.text();
      expect(memberRevoke.status(), revokeBody).toBe(204);

      const revokedSharesResponse = await memberContext.request.get(
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
      await memberContext.close();
      await adminContext.close();
    }
  });
});
