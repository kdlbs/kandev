import { expect, type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import type { UpgradeBackend } from "../fixtures/upgrade-backend";
import type { UpgradeSeed } from "./upgrade-seed";
import { sendUpgradeFollowup, waitForUpgradeTurn } from "./upgrade-api";
import { waitForUpgradeArchive } from "./upgrade-database";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { SessionPage } from "../pages/session-page";

export async function resumeUpgradeTask(
  backend: UpgradeBackend,
  task: { id: string; session_id?: string },
  profileId: string,
  executorProfileId: string,
) {
  const before = task.session_id
    ? (await backend.api.listSessionMessages(task.session_id)).messages.filter((m) =>
        m.content.includes("simple mock response"),
      ).length
    : 0;
  if (task.session_id) {
    const result = await backend.api.wsRequest<{ session_id: string }>(
      "session.recover",
      { task_id: task.id, session_id: task.session_id, action: "resume" },
      90_000,
    );
    expect(result.session_id, "explicit recovery preserves selected conversation").toBe(
      task.session_id,
    );
    await expect
      .poll(
        async () =>
          (await backend.api.listTaskSessions(task.id)).sessions.find(
            (s) => s.id === task.session_id,
          )?.state,
        { timeout: 90_000 },
      )
      .toBe("WAITING_FOR_INPUT");
    await backend.api.addUserMessage(task.id, task.session_id, "/e2e:simple-message");
  } else {
    const result = await backend.api.launchSession(
      {
        task_id: task.id,
        agent_profile_id: profileId,
        executor_profile_id: executorProfileId,
        prompt: "/e2e:simple-message",
      },
      90_000,
    );
    task.session_id = result.session_id;
  }
  await waitForUpgradeTurn(backend.api, task.id, task.session_id, before + 1);
}
export async function exerciseUpgradeLifecycle(backend: UpgradeBackend, seed: UpgradeSeed) {
  const api = backend.api;
  for (const kind of ["pending", "completed", "interrupted"]) {
    await resumeUpgradeTask(backend, seed.tasks[kind], seed.profileId, seed.executorProfileId);
  }
  await resumeUpgradeTask(backend, seed.tasks.waiting, seed.profileId, seed.executorProfileId);
  await sendUpgradeFollowup(api, seed.tasks.waiting.id, seed.tasks.waiting.session_id!);
  const fresh = await api.createTaskWithAgent(
    seed.workspaceId,
    "Upgrade new candidate task",
    seed.profileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.steps[0].id,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.executorProfileId,
    },
  );
  if (!fresh.session_id) throw new Error("candidate new task has no session");
  await waitForUpgradeTurn(api, fresh.id, fresh.session_id);
  await api.addUserMessage(fresh.id, fresh.session_id, "/sleep 300");
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(fresh.id)).sessions.find((s) => s.id === fresh.session_id)
          ?.state,
      { timeout: 30_000 },
    )
    .toBe("RUNNING");
  await api.stopSession({ session_id: fresh.session_id });
  await expect
    .poll(
      async () =>
        (await api.listTaskSessions(fresh.id)).sessions.find((s) => s.id === fresh.session_id)
          ?.state,
      { timeout: 30_000 },
    )
    .toBe("CANCELLED");
  await expect
    .poll(
      async () =>
        (
          await api.wsRequest<{ is_agent_running: boolean }>("task.session.status", {
            task_id: fresh.id,
            session_id: fresh.session_id,
          })
        ).is_agent_running,
      { timeout: 30_000, message: "stopped execution has left runtime tracking" },
    )
    .toBe(false);
  await resumeUpgradeTask(backend, fresh, seed.profileId, seed.executorProfileId);
  await api.moveTask(fresh.id, seed.workflowId, seed.steps[1].id);
  await expect
    .poll(async () => (await api.listTaskSessions(fresh.id)).sessions.length, { timeout: 30_000 })
    .toBe(2);
  const next = (await api.listTaskSessions(fresh.id)).sessions.find(
    (s) => s.id !== fresh.session_id,
  )!;
  expect(next.agent_profile_id).toBe(seed.reviewProfileId);
  expect((await api.getTask(fresh.id)).workflow_step_id).toBe(seed.steps[1].id);
  await waitForUpgradeTurn(api, fresh.id, next.id);
  await api.moveTask(fresh.id, seed.workflowId, seed.steps.at(-1)!.id);
  await api.updateTaskState(fresh.id, "COMPLETED");
  expect((await api.getTask(fresh.id)).state).toBe("COMPLETED");
  expect((await api.getTask(fresh.id)).workflow_step_id).toBe(seed.steps.at(-1)!.id);
  await api.archiveTask(fresh.id);
  expect((await api.getTask(fresh.id)).archived_at).toBeTruthy();
  await waitForUpgradeArchive(backend, fresh.id);
  await api.unarchiveTask(fresh.id);
  await api.wsRequest("task.plan.create", {
    task_id: fresh.id,
    title: "Candidate plan",
    content: "Preserved candidate plan",
  });
  seed.tasks.new = fresh;
  await assertUpgradeLifecycleResult(backend, seed);
  const environment = await api.getTaskEnvironment(seed.tasks.waiting.id);
  const worktree = environment?.repos?.[0]?.worktree_path ?? environment?.worktree_path;
  if (!worktree) throw new Error("resumed worktree path missing");
  expect(fs.readFileSync(path.join(worktree, "upgrade-preserved.txt"), "utf8")).toBe(
    "UPGRADE_PRESERVED_FILE\n",
  );
}
export async function assertUpgradeLifecycleResult(backend: UpgradeBackend, seed: UpgradeSeed) {
  const fresh = seed.tasks.new;
  const task = await backend.api.getTask(fresh.id);
  expect(task.state).toBe("COMPLETED");
  expect(task.workflow_step_id).toBe(seed.steps.at(-1)!.id);
  expect(task.archived_at ?? null).toBeNull();
  const sessions = (await backend.api.listTaskSessions(fresh.id)).sessions;
  expect(sessions).toHaveLength(2);
  expect(sessions.find((session) => session.id !== fresh.session_id)?.agent_profile_id).toBe(
    seed.reviewProfileId,
  );
}

export async function browserUpgradeFollowup(
  page: Page,
  backend: UpgradeBackend,
  seed: UpgradeSeed,
) {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("response", (response) => {
    if (response.url().startsWith(backend.baseUrl + "/api/") && response.status() >= 400)
      errors.push(`${response.status()} ${response.url()}`);
  });
  const task = seed.tasks.waiting;
  await page.goto(`${backend.baseUrl}/t/${task.id}`);
  const session = new SessionPage(page);
  if (backend.info.project.name === "upgrade-mobile") {
    await session.waitForLoad();
  } else {
    await session.showSessionContext();
  }
  await session.waitForChatIdle({ timeout: 90_000, requireEditable: true });
  await session.expectChatResponseVisible("simple mock response", 0);
  const before = (await backend.api.listSessionMessages(task.session_id!)).messages.filter((m) =>
    m.content.includes("simple mock response"),
  ).length;
  if (backend.info.project.name === "upgrade-mobile") {
    await session.sendMessageViaButton("/e2e:simple-message");
  } else {
    await session.sendMessage("/e2e:simple-message");
  }
  await waitForUpgradeTurn(backend.api, task.id, task.session_id!, before + 1);
  await session.expectChatResponseVisible("simple mock response", before);
  expect((await backend.api.listTaskSessions(task.id)).sessions.find((s) => s.is_primary)?.id).toBe(
    task.session_id,
  );
  expect((await backend.api.getTaskPlan(task.id))?.content).toBe("Preserved waiting plan");
  if (backend.info.project.name === "upgrade-mobile") {
    await page.getByRole("navigation").getByRole("button", { name: "Plan", exact: true }).tap();
  } else {
    await session.activeChat().getByTestId("plan-mode-toggle-button").click();
  }
  await expect(session.planPanel).toContainText("Preserved waiting plan");
  await assertNoDocumentHorizontalOverflow(page);
  expect(errors).toEqual([]);
}
