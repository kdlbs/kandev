import { expect, test } from "../../fixtures/test-base";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import {
  managedRuntimeStartupAttemptFile,
  managedRuntimeStartupSentinels,
  prepareManagedRuntimeProfile,
  restoreE2EAgentRegistry,
} from "../../helpers/managed-runtime-recovery";
import { waitForSessionDone, waitForSessionState } from "../../helpers/session";
import { dwell } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";

test.describe("managed runtime startup retry", () => {
  test("retries a transient npm failure in the same session and preserves shared files", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
    test.setTimeout(180_000);

    const launchId = randomUUID();
    let profileId = "";
    let hostFixturePath = "";
    try {
      const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
        startupMode: "transient",
        launchId,
        hostSubprocess: true,
      });
      profileId = prepared.profile.id;
      hostFixturePath = prepared.hostFixturePath ?? "";

      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Managed runtime transient startup retry",
        prepared.profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("Retrying agent startup (attempt 2 of 2)", { exact: true }),
      ).toBeVisible({ timeout: 30_000 });

      await waitForSessionDone(
        apiClient,
        task.id,
        task.session_id,
        "Wait for managed runtime retry to finish in the original session",
      );
      const { sessions } = await apiClient.listTaskSessions(task.id);
      expect(sessions.map((candidate) => candidate.id)).toContain(task.session_id);
      const { messages } = await apiClient.listSessionMessages(task.session_id);
      expect(
        messages.filter(
          (message) => message.author_type === "user" && message.content === "/e2e:simple-message",
        ),
      ).toHaveLength(1);
      expect(
        messages.some(
          (message) =>
            message.author_type === "agent" && message.content.includes("simple mock response"),
        ),
      ).toBe(true);
      expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("2");
      const sentinels = managedRuntimeStartupSentinels(prepared.packageSpec, launchId);
      expect(fs.readFileSync(sentinels.selected, "utf8")).toBe("preserved\n");
      expect(fs.readFileSync(sentinels.sibling, "utf8")).toBe("preserved\n");
      await expect(
        session.activeChat().getByTestId("managed-runtime-startup-recovery"),
      ).toHaveCount(0);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
      await restoreE2EAgentRegistry(backend);
    }
  });

  test("shows one accurate recovery card after both transient attempts fail", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
    test.setTimeout(180_000);

    const launchId = randomUUID();
    let profileId = "";
    let hostFixturePath = "";
    try {
      const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
        startupMode: "repeat-failure",
        launchId,
        hostSubprocess: true,
      });
      profileId = prepared.profile.id;
      hostFixturePath = prepared.hostFixturePath ?? "";
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Managed runtime startup exhaustion",
        prepared.profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "Wait for managed runtime startup recovery",
      });
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const failedSession = sessions.find((candidate) => candidate.id === task.session_id);
      const lastAgentError = failedSession?.metadata?.last_agent_error as
        | Record<string, unknown>
        | undefined;
      expect(lastAgentError).toMatchObject({
        code: "managed_runtime_startup",
        startup_reason: "npm_transient",
        startup_attempts: 2,
        startup_npm_code: "ECONNRESET",
      });
      const { messages } = await apiClient.listSessionMessages(task.session_id);
      const recoveryMessage = messages.find(
        (message) => message.metadata?.recovery_actions === true,
      );
      expect(recoveryMessage?.metadata).toMatchObject({
        failure_kind: "managed_runtime_startup",
        startup_reason: "npm_transient",
        startup_attempts: 2,
      });

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const card = session.activeChat().getByTestId("session-recovery-card");
      await expect(card).toBeVisible({ timeout: 45_000 });
      await expect(
        card.getByRole("heading", { name: "npm could not prepare the runtime" }),
      ).toBeVisible();
      await expect(card).toContainText(
        "Kandev retried the same runtime once after a temporary npm failure (2 attempts total).",
      );
      await expect(card).not.toContainText("agent process exited");
      await expect(card.getByTestId("managed-runtime-npm-retry-button")).toHaveCount(1);
      await expect(card.locator("details")).not.toHaveAttribute("open");
      expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("2");
      expect(sessions.map((candidate) => candidate.id)).toContain(task.session_id);
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
      await restoreE2EAgentRegistry(backend);
    }
  });

  test("does not replace a process after npm reports a permanent authorization failure", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
    test.setTimeout(180_000);

    const launchId = randomUUID();
    let profileId = "";
    let hostFixturePath = "";
    try {
      const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
        startupMode: "permanent",
        launchId,
        hostSubprocess: true,
      });
      profileId = prepared.profile.id;
      hostFixturePath = prepared.hostFixturePath ?? "";
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Managed runtime permanent startup refusal",
        prepared.profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "WAITING_FOR_INPUT",
        message: "Wait for permanent npm refusal",
      });
      const { sessions } = await apiClient.listTaskSessions(task.id);
      const failedSession = sessions.find((candidate) => candidate.id === task.session_id);
      const lastAgentError = failedSession?.metadata?.last_agent_error as
        | Record<string, unknown>
        | undefined;
      expect(lastAgentError).toMatchObject({
        code: "managed_runtime_startup",
        startup_reason: "permanent_npm_error",
        startup_attempts: 1,
        startup_npm_code: "E401",
      });
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      const card = session.activeChat().getByTestId("session-recovery-card");
      await expect(card).toBeVisible();
      await expect(
        card.getByRole("heading", { name: "npm could not prepare the runtime" }),
      ).toBeVisible();
      await expect(card).toContainText("npm refused to prepare this runtime");
      expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("1");
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
      await restoreE2EAgentRegistry(backend);
    }
  });

  test("cancelling during retry backoff prevents the replacement launch", async ({
    apiClient,
    backend,
    seedData,
    testPage,
  }) => {
    test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
    test.setTimeout(180_000);

    const launchId = randomUUID();
    let profileId = "";
    let hostFixturePath = "";
    try {
      const prepared = await prepareManagedRuntimeProfile(apiClient, backend, {
        startupMode: "transient",
        launchId,
        hostSubprocess: true,
      });
      profileId = prepared.profile.id;
      hostFixturePath = prepared.hostFixturePath ?? "";
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Cancel managed runtime retry",
        prepared.profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      if (!task.session_id) throw new Error("managed runtime task did not return a session ID");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await expect(
        session.activeChat().getByText("Retrying agent startup (attempt 2 of 2)", { exact: true }),
      ).toBeVisible({ timeout: 30_000 });
      expect(
        (
          await apiClient.stopSession({
            session_id: task.session_id,
            reason: "cancel retry backoff",
            force: true,
          })
        ).success,
      ).toBe(true);
      await waitForSessionState(apiClient, {
        taskId: task.id,
        sessionId: task.session_id,
        expectedState: "CANCELLED",
        message: "Wait for startup cancellation",
      });
      expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("1");
      await dwell(
        3_500,
        "negative-assertion",
        "the cancelled launch must not start a replacement during the retry window",
      );
      expect(fs.readFileSync(managedRuntimeStartupAttemptFile(launchId), "utf8").trim()).toBe("1");
    } finally {
      if (profileId) await apiClient.deleteAgentProfile(profileId, true).catch(() => undefined);
      if (hostFixturePath) fs.rmSync(hostFixturePath, { force: true });
      await restoreE2EAgentRegistry(backend);
    }
  });
});
