import type { Browser, BrowserContext } from "@playwright/test";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { acceptInvite, createInviteToken, setupAdmin } from "../../helpers/auth";
import { waitForSessionState } from "../../helpers/session";
import {
  adoptionWS,
  liveAdoptionConsumers,
  prepareIndependentAdoptionClone,
  readAdoptionIdentity,
} from "../../helpers/workspace-adoption";

for (const authenticated of [false, true]) {
  test(`independent adoption preserves a native session across restart (auth ${authenticated})`, async ({
    apiClient,
    seedData,
    backend,
    browser,
  }) => {
    test.setTimeout(120_000);
    const context = await browser.newContext({ baseURL: backend.baseUrl });
    let restoreAdministration: ((adopted: boolean) => void) | undefined;
    let adopted = false;
    try {
      if (!authenticated) {
        for (const [method, suffix] of [
          ["POST", "preview"],
          ["POST", "apply"],
          ["GET", randomUUID()],
        ]) {
          const result = await context.request.fetch(
            `${backend.baseUrl}/api/v1/tasks/unknown/workspace-adoption/${suffix}`,
            {
              method,
              headers: { Authorization: "Bearer untrusted-agent" },
              ...(method === "POST" ? { data: {} } : {}),
            },
          );
          expect(result.status(), await result.text()).toBe(403);
        }
      }
      await apiClient.saveUserSettings({ prevent_auto_start_agent_on_open: true });
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Independent adoption",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.worktreeExecutorProfileId,
        },
      );
      expect(task.session_id).toBeTruthy();
      const sessionId = task.session_id!;
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId,
        expectedState: "WAITING_FOR_INPUT",
        message: "Initial native turn must finish",
      });
      expect(liveAdoptionConsumers(backend.tmpDir, sessionId)).toBeGreaterThan(0);
      const identity = readAdoptionIdentity(backend.tmpDir, sessionId);
      expect(identity.checkout_branch).toBe("");
      const { messages: initialMessages } = await apiClient.listSessionMessages(sessionId);
      expect(
        initialMessages.some((message) => message.content.includes("simple mock response")),
      ).toBe(true);
      await backend.restart({ KANDEV_FEATURES_AUTH: String(authenticated) });
      await expect.poll(() => liveAdoptionConsumers(backend.tmpDir, sessionId)).toBe(0);
      const clone = prepareIndependentAdoptionClone(
        backend.tmpDir,
        seedData.repositoryPath,
        identity,
      );
      restoreAdministration = clone.restoreAdministration;
      if (authenticated) {
        await setupAdmin(context, backend.baseUrl, {
          email: "adoption-admin@e2e.dev",
          password: "adoption-admin-password",
        });
        await assertAdoptionAuthentication(browser, context, backend.baseUrl, task.id);
      }
      const endpoint = `${backend.baseUrl}/api/v1/tasks/${task.id}/workspace-adoption`;
      if (!authenticated) {
        for (const [method, suffix] of [
          ["POST", "preview"],
          ["POST", "apply"],
          ["GET", randomUUID()],
        ]) {
          const refused = await context.request.fetch(`${endpoint}/${suffix}`, {
            method,
            ...(method === "POST"
              ? {
                  data: {
                    session_id: sessionId,
                    slot_id: identity.slot_id,
                    checkout_path: clone.candidate,
                  },
                }
              : {}),
          });
          expect(refused.status(), await refused.text()).toBe(403);
        }
        expect(readAdoptionIdentity(backend.tmpDir, sessionId)).toEqual(identity);
        expect(fs.readFileSync(path.join(identity.worktree_path, ".git"))).toEqual(clone.marker);
        return;
      }
      const previewResponse = await context.request.post(`${endpoint}/preview`, {
        data: { session_id: sessionId, slot_id: identity.slot_id, checkout_path: clone.candidate },
      });
      expect(previewResponse.status(), await previewResponse.text()).toBe(200);
      const preview = await previewResponse.json();
      const operationId = randomUUID();
      const apply = await context.request.post(`${endpoint}/apply`, {
        data: { preview, operation_id: operationId },
      });
      expect(apply.status(), await apply.text()).toBe(200);
      adopted = true;
      const receipt = await apply.json();
      const replay = await context.request.post(`${endpoint}/apply`, {
        data: { preview, operation_id: operationId },
      });
      expect(replay.status(), await replay.text()).toBe(200);
      expect(await replay.json()).toEqual(receipt);
      expect(readAdoptionIdentity(backend.tmpDir, sessionId)).toEqual({
        ...identity,
        workspace_path: clone.candidate,
        worktree_path: clone.candidate,
        worktree_source_clone_path: clone.candidate,
        worktree_source_common_dir: path.join(clone.candidate, ".git"),
      });
      expect(fs.readFileSync(path.join(identity.worktree_path, ".git"))).toEqual(clone.marker);
      expect(fs.readFileSync(path.join(clone.candidate, ".git", "index"))).toEqual(clone.index);
      await backend.restart({ KANDEV_FEATURES_AUTH: String(authenticated) });
      const restoredReceipt = await context.request.get(`${endpoint}/${operationId}`);
      expect(restoredReceipt.status(), await restoredReceipt.text()).toBe(200);
      expect(await restoredReceipt.json()).toEqual(receipt);
      const page = await context.newPage();
      await page.goto(`${backend.baseUrl}/health`);
      const resumed = await adoptionWS<{ session_id: string; agent_execution_id: string }>(
        page,
        backend.baseUrl,
        "session.launch",
        {
          task_id: task.id,
          session_id: sessionId,
          intent: "resume",
        },
      );
      expect(resumed.session_id).toBe(sessionId);
      expect(resumed.agent_execution_id).toBeTruthy();
      await expect
        .poll(() => liveAdoptionConsumers(backend.tmpDir, sessionId), { timeout: 30_000 })
        .toBeGreaterThan(0);
      await adoptionWS(page, backend.baseUrl, "message.add", {
        task_id: task.id,
        session_id: sessionId,
        content: "/e2e:diff-update-modify",
      });
      const changedFile = path.join(clone.candidate, "diff_update_test.txt");
      await expect
        .poll(() => (fs.existsSync(changedFile) ? fs.readFileSync(changedFile, "utf8") : ""), {
          timeout: 30_000,
        })
        .toContain("SECOND_MODIFICATION");
      const file = await adoptionWS<{ content: string }>(
        page,
        backend.baseUrl,
        "workspace.file.get",
        {
          session_id: sessionId,
          path: "adoption-sentinel.txt",
        },
      );
      expect(file.content).toBe(clone.sentinel);
      expect(readAdoptionIdentity(backend.tmpDir, sessionId)).toEqual({
        ...identity,
        workspace_path: clone.candidate,
        worktree_path: clone.candidate,
        worktree_source_clone_path: clone.candidate,
        worktree_source_common_dir: path.join(clone.candidate, ".git"),
      });
      const receiptAfterResume = await context.request.get(`${endpoint}/${operationId}`);
      expect(receiptAfterResume.status(), await receiptAfterResume.text()).toBe(200);
      expect(await receiptAfterResume.json()).toEqual(receipt);
      const messagesResponse = await context.request.get(
        `${backend.baseUrl}/api/v1/task-sessions/${sessionId}/messages?limit=100`,
      );
      expect(messagesResponse.ok(), await messagesResponse.text()).toBe(true);
      const { messages } = (await messagesResponse.json()) as {
        messages: Array<{ id: string; content: string }>;
      };
      for (const old of initialMessages)
        expect(
          messages.some((message) => message.id === old.id && message.content === old.content),
        ).toBe(true);
      expect(fs.readFileSync(path.join(identity.worktree_path, ".git"))).toEqual(clone.marker);
      await backend.restart({ KANDEV_FEATURES_AUTH: String(authenticated) });
      const receiptAfterResumedRestart = await context.request.get(`${endpoint}/${operationId}`);
      expect(receiptAfterResumedRestart.status(), await receiptAfterResumedRestart.text()).toBe(
        200,
      );
      expect(await receiptAfterResumedRestart.json()).toEqual(receipt);
    } finally {
      await context.close();
      restoreAdministration?.(adopted);
      await backend.restart();
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]);
    }
  });
}

async function assertAdoptionAuthentication(
  browser: Browser,
  admin: BrowserContext,
  baseUrl: string,
  taskId: string,
) {
  const anonymous = await browser.newContext();
  const member = await browser.newContext();
  try {
    const token = await createInviteToken(admin, baseUrl, {
      role: "member",
      email: "adoption-member@e2e.dev",
    });
    await acceptInvite(member, baseUrl, token, {
      email: "adoption-member@e2e.dev",
      password: "adoption-member-password",
    });
    for (const [method, suffix] of [
      ["POST", "preview"],
      ["POST", "apply"],
      ["GET", randomUUID()],
    ]) {
      const url = `${baseUrl}/api/v1/tasks/${taskId}/workspace-adoption/${suffix}`;
      for (const [context, status] of [
        [anonymous, 401],
        [member, 403],
      ] as const) {
        const result = await context.request.fetch(url, {
          method,
          ...(method === "POST" ? { data: {} } : {}),
        });
        expect(result.status(), await result.text()).toBe(status);
      }
      const agent = await anonymous.request.fetch(url, {
        method,
        headers: { Authorization: "Bearer untrusted-agent" },
        ...(method === "POST" ? { data: {} } : {}),
      });
      expect(agent.status(), await agent.text()).toBe(401);
    }
  } finally {
    await anonymous.close();
    await member.close();
  }
}
