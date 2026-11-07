import { randomUUID } from "node:crypto";
import fs from "node:fs";
import { DatabaseSync } from "./node-sqlite";
import path from "node:path";
import { expect, test } from "../fixtures/test-base";
import { waitForSessionState } from "./session";
import {
  managedRuntimeExecutionCacheKey,
  prepareIncompleteManagedRuntimeProfile,
} from "./managed-runtime-recovery";
import { SessionPage } from "../pages/session-page";

export function registerManagedNpmCacheRecoveryTests() {
  test.describe("managed npm incomplete cache recovery", () => {
    test.describe.configure({ retries: 0 });

    for (const runtimeFamily of ["v1", "v2"] as const) {
      test(`prepares the selected ${runtimeFamily} runtime and resumes the same failed conversation`, async ({
        apiClient,
        backend,
        seedData,
        testPage,
      }) => {
        test.skip(process.platform === "win32", "the deterministic npx fixture is a POSIX script");
        test.setTimeout(240_000);

        const launchId = randomUUID();
        let profileId = "";
        let preparedRoot = "";
        let fixture: Awaited<ReturnType<typeof prepareIncompleteManagedRuntimeProfile>> | undefined;
        let taskId = "";
        let sessionId = "";
        try {
          fixture = await prepareIncompleteManagedRuntimeProfile(apiClient, backend, {
            startupMode: "missing-root-manifest",
            launchId,
            hostSubprocess: true,
            runtimeFamily,
          });
          profileId = fixture.profile.id;
          const task = await apiClient.createTaskWithAgent(
            seedData.workspaceId,
            "Managed npm incomplete-cache recovery",
            fixture.profile.id,
            {
              description: "/e2e:simple-message",
              workflow_id: seedData.workflowId,
              workflow_step_id: seedData.startStepId,
              repository_ids: [seedData.repositoryId],
            },
          );
          taskId = task.id;
          if (!task.session_id) throw new Error("managed runtime task did not return a session ID");
          sessionId = task.session_id;

          await testPage.goto(`/t/${task.id}`);
          const session = new SessionPage(testPage);
          await session.waitForLoad();
          await waitForSessionState(apiClient, {
            taskId,
            sessionId,
            expectedState: "WAITING_FOR_INPUT",
            message: "private runtime should initialize",
            timeout: 45_000,
          });
          await session.expectChatResponseVisible("simple mock response", 0, { timeout: 15_000 });
          await waitForSessionState(apiClient, {
            taskId,
            sessionId,
            expectedState: "WAITING_FOR_INPUT",
            message: "managed runtime should accept one prompt after private preparation",
          });

          const { messages } = await apiClient.listSessionMessages(sessionId);
          expect(
            messages.filter(
              (message) =>
                message.author_type === "user" && message.content?.trim() === "/e2e:simple-message",
            ),
          ).toHaveLength(1);
          await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);
          await expect(testPage.getByTestId("failed-session-banner")).toHaveCount(0);
          await session.waitForDirectInput();

          const npmLog = fixture.npmInvocationLog;
          if (!npmLog)
            throw new Error("host npm invocation capture is required for private preparation");
          await expect.poll(() => fs.existsSync(npmLog)).toBe(true);
          const args =
            fs
              .readFileSync(npmLog, "utf8")
              .trim()
              .split(/\r?\n/)
              .filter(Boolean)
              .map((invocation) => JSON.parse(invocation) as string[])
              .filter((invocation) => invocation.some((arg) => arg.startsWith("--package=")))
              .at(-1) ?? [];
          expect(args).toContain("exec");
          expect(args).toContain("--yes");
          expect(args).toContain("--prefer-online");
          expect(args).toContain(`--package=${fixture.packageSpec}`);
          const prefixIndex = args.indexOf("--prefix");
          const cacheIndex = args.indexOf("--cache");
          expect(prefixIndex).toBeGreaterThanOrEqual(0);
          expect(cacheIndex).toBeGreaterThanOrEqual(0);
          const privateProject = args[prefixIndex + 1];
          const privateCache = args[cacheIndex + 1];
          preparedRoot = path.dirname(privateProject);
          expect(privateProject).toContain("/kandev-managed-runtime-");
          expect(privateCache).toContain(preparedRoot);
          expect(fixture.registry.requests()).toBeGreaterThan(0);
          const privateTree = path.join(
            privateCache,
            "_npx",
            managedRuntimeExecutionCacheKey(fixture.packageSpec),
          );
          expect(fs.existsSync(path.join(privateTree, "package.json"))).toBe(true);
          expect(fs.existsSync(path.join(privateTree, "node_modules", ".bin", "opencode"))).toBe(
            true,
          );
          const packageSeparator = fixture.packageSpec.lastIndexOf("@");
          const packageName = fixture.packageSpec.slice(0, packageSeparator);
          const sharedPackage = path.join(
            fixture.sharedRoot,
            "node_modules",
            ...packageName.split("/"),
            "package.json",
          );
          expect(JSON.parse(fs.readFileSync(sharedPackage, "utf8"))).toMatchObject({
            name: packageName,
            version: fixture.packageSpec.slice(packageSeparator + 1),
          });
          expect(fs.existsSync(path.join(fixture.sharedRoot, "package.json"))).toBe(false);
          expect(fs.readFileSync(fixture.sentinels.selected, "utf8")).toBe("preserved\n");
          expect(fs.readFileSync(fixture.sentinels.sibling, "utf8")).toBe("preserved\n");

          await testPage.goto("/");
          const beforeRestart = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
          const token = beforeRestart
            .prepare("SELECT resume_token FROM executors_running WHERE session_id = ?")
            .get(sessionId) as { resume_token: string };
          beforeRestart.close();
          expect(token.resume_token).toBeTruthy();
          await backend.restart();
          const failedSession = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
          try {
            failedSession.exec("PRAGMA busy_timeout=5000");
            failedSession
              .prepare(
                "UPDATE task_sessions SET state = 'FAILED', error_message = 'ACP initialize failed' WHERE id = ?",
              )
              .run(sessionId);
          } finally {
            failedSession.close();
          }
          await testPage.goto(`/t/${taskId}`);
          await session.waitForLoad();
          await waitForSessionState(apiClient, {
            taskId,
            sessionId,
            expectedState: "WAITING_FOR_INPUT",
            message: "saved failed session should resume automatically",
            timeout: 60_000,
          });
          await session.waitForDirectInput();
          await expect(
            testPage.getByText("Workspace restored in read-only mode", { exact: true }),
          ).toHaveCount(0);
          const resumed = await apiClient.listTaskSessions(taskId);
          expect(resumed.sessions.map((item) => item.id)).toEqual([sessionId]);
          const afterRestart = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
          const resumedToken = afterRestart
            .prepare("SELECT resume_token FROM executors_running WHERE session_id = ?")
            .get(sessionId) as { resume_token: string };
          afterRestart.close();
          expect(resumedToken.resume_token).toBe(token.resume_token);
          await session.sendMessageViaButton("/e2e:simple-message");
          await session.expectChatResponseVisible("simple mock response", 1, { timeout: 60_000 });
          const afterFollowup = await apiClient.listSessionMessages(sessionId);
          expect(
            afterFollowup.messages.filter(
              (message) =>
                message.author_type === "user" && message.content?.trim() === "/e2e:simple-message",
            ),
          ).toHaveLength(2);
        } catch (error) {
          await test.info().attach("managed-runtime-recovery-error", {
            body: error instanceof Error ? (error.stack ?? error.message) : String(error),
            contentType: "text/plain",
          });
          await test.info().attach("managed-runtime-backend-log", {
            path: backend.logPath,
            contentType: "text/plain",
          });
          throw error;
        } finally {
          if (sessionId) {
            await apiClient
              .stopSession({
                session_id: sessionId,
                reason: "managed npm E2E cleanup",
                force: true,
              })
              .catch(() => undefined);
            if (taskId) {
              await waitForSessionState(apiClient, {
                taskId,
                sessionId,
                expectedState: "CANCELLED",
                message: "managed runtime E2E process should stop before cleanup",
                timeout: 30_000,
              }).catch(() => undefined);
            }
          }
          if (fixture) {
            await testPage.goto("/");
            await fixture.releaseBackendEnv();
            if (preparedRoot) await expect.poll(() => fs.existsSync(preparedRoot)).toBe(false);
            if (profileId) await apiClient.deleteAgentProfile(profileId, true);
            await fixture.registry.close().catch(() => undefined);
            fs.rmSync(fixture.cacheRoot, { recursive: true, force: true });
            for (const fixturePath of fixture.hostFixturePaths)
              fs.rmSync(fixturePath, { force: true });
            if (fixture.npmInvocationLog) fs.rmSync(fixture.npmInvocationLog, { force: true });
          }
        }
      });
    }
  });
}
