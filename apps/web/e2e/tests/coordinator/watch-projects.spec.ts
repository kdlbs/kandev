// The Projects scope of a coordinator's watches: a task outside the selected
// projects never reaches Needs you or its count, and the settings write rejects
// entries that are not in the workspace
// (docs/specs/coordinator/requirements/permissions.md, REQ-COORDINATOR-PERMISSIONS-005).
import { test, expect } from "../../fixtures/test-base";
import { waitForSessionState } from "../../helpers/session";
import { waitForHttp } from "../../helpers/causal-waits";
import type { ApiClient } from "../../helpers/api-client";
import {
  linkToCoordinatorNeedsYou,
  linkToCoordinatorSettings,
} from "../../../lib/coordinator/links";
import { PHASE31_ENV, createLocalRepository } from "./watch-projects-fixture";

const SETTINGS_PATH = /\/coordinators\/[^/]+\/settings$/;

type Seed = {
  workspaceId: string;
  agentProfileId: string;
  workflowId: string;
  repositoryId: string;
  startStepId: string;
  worktreeExecutorProfileId: string;
};

async function blockedTask(apiClient: ApiClient, seed: Seed, title: string, repoId: string) {
  const task = await apiClient.createTaskWithAgent(seed.workspaceId, title, seed.agentProfileId, {
    description: "/e2e:clarification",
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [repoId],
  });
  if (!task.session_id) throw new Error("expected an active session for the clarification task");
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    message: "clarification session should block before Needs you is opened",
    timeout: 60_000,
  });
  return task;
}

test.describe("Coordinator watch projects", () => {
  test("narrowing to one project hides the other project's task and its count", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const release = await backend.useEnv(PHASE31_ENV);
    try {
      const outside = await createLocalRepository(
        apiClient,
        backend.tmpDir,
        seedData.workspaceId,
        "Elsewhere",
      );
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Project Watcher",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const inScope = await blockedTask(
        apiClient,
        seedData,
        "In scope task",
        seedData.repositoryId,
      );
      const outOfScope = await blockedTask(apiClient, seedData, "Out of scope task", outside.id);

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId(`needs-you-item-${inScope.id}`)).toBeVisible();
      await expect(testPage.getByTestId(`needs-you-item-${outOfScope.id}`)).toBeVisible();
      await expect(testPage.getByTestId("count-needs-you")).toContainText("2");

      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=watches`,
      );
      await testPage.getByTestId("watches-projects-all").click();
      await testPage.getByTestId(`watches-project-toggle-${outside.id}`).click();
      const put = waitForHttp(testPage, "PUT", SETTINGS_PATH);
      await testPage.getByRole("button", { name: "Save changes" }).click();
      expect((await put).status()).toBe(200);

      await testPage.goto(linkToCoordinatorNeedsYou(seedData.workspaceId, coordinator.id));
      await expect(testPage.getByTestId(`needs-you-item-${inScope.id}`)).toBeVisible();
      await expect(testPage.getByTestId(`needs-you-item-${outOfScope.id}`)).toHaveCount(0);
      await expect(testPage.getByTestId("count-needs-you")).toContainText("1");
    } finally {
      await release();
    }
  });

  test("the settings write validates entries and the coordinator read names the projects", async ({
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const release = await backend.useEnv(PHASE31_ENV);
    try {
      const set = await apiClient.createRepositorySet(seedData.workspaceId, "Platform", [
        seedData.repositoryId,
      ]);
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Project Validator",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const base = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;

      const empty = await apiClient.rawRequest("PUT", `${base}/settings`, {
        projects: { scope: "selected", entries: [], include_no_repository: false },
      });
      expect(empty.status).toBe(400);
      expect(((await empty.json()) as { code?: string }).code).toBe("projects_empty");

      const foreign = await apiClient.rawRequest("PUT", `${base}/settings`, {
        projects: {
          scope: "selected",
          entries: [{ kind: "repository_set", id: "not-in-this-workspace" }],
          include_no_repository: false,
        },
      });
      expect(foreign.status).toBe(400);
      expect(((await foreign.json()) as { code?: string }).code).toBe("projects_foreign_entry");

      const saved = await apiClient.rawRequest("PUT", `${base}/settings`, {
        projects: {
          scope: "selected",
          entries: [{ kind: "repository_set", id: set.id }],
          include_no_repository: true,
        },
      });
      expect(saved.status).toBe(200);

      const read = await apiClient.rawRequest("GET", base);
      const body = (await read.json()) as {
        watches: {
          projects: {
            scope: string;
            repository_ids: string[];
            include_no_repository: boolean;
            names: string[];
          };
        };
      };
      expect(body.watches.projects).toEqual({
        scope: "selected",
        repository_ids: [seedData.repositoryId],
        include_no_repository: true,
        names: ["Platform"],
      });
    } finally {
      await release();
    }
  });

  test("Save stays blocked while the project listing fails to load", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const release = await backend.useEnv(PHASE31_ENV);
    try {
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Listing Failure",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const base = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
      const seeded = await apiClient.rawRequest("PUT", `${base}/settings`, {
        projects: {
          scope: "selected",
          entries: [{ kind: "repository", id: seedData.repositoryId }],
          include_no_repository: false,
        },
      });
      expect(seeded.status).toBe(200);

      await testPage.route(/\/api\/v1\/workspaces\/[^/]+\/repositories(\?.*)?$/, (route) =>
        route.abort(),
      );
      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=watches`,
      );
      await expect(testPage.getByTestId("watches-projects-failed")).toBeVisible();
      await testPage.getByTestId("watches-keep-one-project").waitFor({ state: "detached" });
      await expect(testPage.getByRole("button", { name: "Save changes" })).toHaveCount(0);
    } finally {
      await release();
    }
  });

  test("a rejected foreign entry is pruned from the draft after the listing refreshes", async ({
    testPage,
    apiClient,
    backend,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const release = await backend.useEnv(PHASE31_ENV);
    try {
      const other = await createLocalRepository(
        apiClient,
        backend.tmpDir,
        seedData.workspaceId,
        "Prune Other",
      );
      const doomed = await apiClient.createRepositorySet(seedData.workspaceId, "Doomed", [
        other.id,
      ]);
      const coordinator = await apiClient.createCoordinator(seedData.workspaceId, {
        name: "Prune Watcher",
        agent_profile_id: seedData.agentProfileId,
        executor_profile_id: seedData.worktreeExecutorProfileId,
        task_agent_profile_id: seedData.agentProfileId,
        task_executor_profile_id: seedData.worktreeExecutorProfileId,
      });
      const base = `/api/v1/workspaces/${seedData.workspaceId}/coordinators/${coordinator.id}`;
      const seeded = await apiClient.rawRequest("PUT", `${base}/settings`, {
        projects: {
          scope: "selected",
          entries: [{ kind: "repository", id: seedData.repositoryId }],
          include_no_repository: false,
        },
      });
      expect(seeded.status).toBe(200);

      await testPage.goto(
        `${linkToCoordinatorSettings(seedData.workspaceId, coordinator.id)}?section=watches`,
      );
      const doomedRow = testPage.getByTestId(`watches-project-repository_set-${doomed.id}`);
      await expect(doomedRow).toBeVisible();
      await testPage.getByTestId(`watches-project-toggle-${doomed.id}`).click();
      await expect(doomedRow).toContainText("In scope");
      await apiClient.deleteRepositorySet(doomed.id);

      const rejected = waitForHttp(testPage, "PUT", SETTINGS_PATH);
      await testPage.getByRole("button", { name: "Save changes" }).click();
      expect((await rejected).status()).toBe(400);

      await expect(
        testPage.getByRole("alert").filter({ hasText: "does not belong to this workspace" }),
      ).toBeVisible();
      await expect(doomedRow).toHaveCount(0);
      await expect(
        testPage.getByTestId(`watches-project-repository-${seedData.repositoryId}`),
      ).toContainText("In scope");
    } finally {
      await release();
    }
  });
});
