import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Browser, type BrowserContext, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import { login, type Credentials } from "./auth";
import { DatabaseSync } from "./node-sqlite";
import { reportFixtureUsage, usageTarget, waitForUsage } from "./usage-history";

export const USAGE_AUTH_DATABASE = "kandev-auth-usage-history.db";

export type UsageTenant = {
  org: { id: string; slug: string };
  context: BrowserContext;
  page: Page;
  workspace: string;
  task: string;
  binding: string;
  owner: string;
  unit: string;
};

export async function createUsageTenant(
  browser: Browser,
  backend: BackendContext,
  operator: BrowserContext,
  slug: string,
  tokens: string,
): Promise<UsageTenant> {
  const created = await operator.request.post("/api/v1/instance/orgs", {
    data: { name: slug, slug },
  });
  expect(created.status(), await created.text()).toBe(200);
  const org = (await created.json()) as { id: string; slug: string };
  const credentials: Credentials = {
    email: `${slug}@e2e.dev`,
    password: "usage-adminpass123",
    displayName: slug,
  };
  const admin = await operator.request.post(`/api/v1/instance/orgs/${org.id}/admins`, {
    data: {
      email: credentials.email,
      password: credentials.password,
      display_name: credentials.displayName,
    },
  });
  expect(admin.status(), await admin.text()).toBe(200);
  const context = await browser.newContext({ baseURL: backend.frontendUrl });
  try {
    await login(context, backend.baseUrl, credentials);
    const page = await context.newPage();
    const seeded = await seedAuthenticatedUsage(page, tokens);
    return { org, context, page, ...seeded };
  } catch (error) {
    await context.close();
    throw error;
  }
}

export async function seedAuthenticatedUsage(page: Page, tokens: string) {
  const workspaceResponse = await page.request.post("/api/v1/workspaces", {
    data: { name: "Retained private usage" },
  });
  expect(workspaceResponse.ok(), await workspaceResponse.text()).toBe(true);
  const workspace = (await workspaceResponse.json()) as {
    id: string;
    owner_id: string;
    unit_id: string;
  };
  const workflowResponse = await page.request.post("/api/v1/workflows", {
    data: {
      workspace_id: workspace.id,
      name: "Usage",
      workflow_template_id: "simple",
    },
  });
  expect(workflowResponse.ok(), await workflowResponse.text()).toBe(true);
  const workflow = (await workflowResponse.json()) as { id: string };
  const stepsResponse = await page.request.get(`/api/v1/workflows/${workflow.id}/workflow/steps`);
  expect(stepsResponse.status(), await stepsResponse.text()).toBe(200);
  const { steps } = (await stepsResponse.json()) as {
    steps: { id: string; position: number; is_start_step: boolean }[];
  };
  const ordered = steps.sort((left, right) => left.position - right.position);
  const step = ordered.find((candidate) => candidate.is_start_step) ?? ordered[0];
  expect(step).toBeDefined();
  const taskResponse = await page.request.post("/api/v1/tasks", {
    data: {
      workspace_id: workspace.id,
      title: "Private retained task",
      workflow_id: workflow.id,
      workflow_step_id: step.id,
    },
  });
  expect(taskResponse.ok(), await taskResponse.text()).toBe(true);
  const task = (await taskResponse.json()) as { id: string };
  // The existing guarded session fixture provides a historical idle session.
  // Binding issuance, collector admission, authorization and reporting are real.
  const sessionResponse = await page.request.post("/api/v1/_test/task-sessions", {
    data: {
      task_id: task.id,
      session_id: randomUUID(),
      state: "COMPLETED",
    },
  });
  expect(sessionResponse.ok(), await sessionResponse.text()).toBe(true);
  const target = await usageTarget(page, workspace.id, task.id);
  await reportFixtureUsage(page, workspace.id, {
    subject_binding: target.SubjectBinding,
    item_id: randomUUID(),
    revision: 1,
    model_id: "retained-private-model",
    provider: "private-provider",
    tokens: { input_tokens: tokens, total_tokens: tokens },
    coverage: { OccurrenceAt: new Date().toISOString() },
    quality: { TokenCoverageComplete: true, CostCoverageComplete: false },
  });
  await waitForUsage(page, workspace.id, tokens);
  return {
    workspace: workspace.id,
    task: task.id,
    binding: target.SubjectBinding,
    owner: workspace.owner_id,
    unit: workspace.unit_id,
  };
}

export async function deleteAuthenticatedTask(page: Page, task: string) {
  const preview = await page.request.post("/api/v1/tasks/delete-preflight", {
    data: {
      task_ids: [task],
      cascade: false,
      discard_worktree_changes: false,
    },
  });
  expect(preview.status(), await preview.text()).toBe(200);
  const { confirmation_id: confirmation } = (await preview.json()) as { confirmation_id: string };
  const deleted = await page.request.delete(`/api/v1/tasks/${task}`, {
    headers: { "X-Kandev-Task-Delete-Confirmation": confirmation },
  });
  expect(deleted.status(), await deleted.text()).toBe(200);
}

function authUsageDatabase(backend: BackendContext) {
  if (!/^kandev-e2e-\d+-/.test(path.basename(backend.tmpDir)))
    throw new Error("Usage fixture requires a disposable E2E database");
  return new DatabaseSync(path.join(backend.tmpDir, USAGE_AUTH_DATABASE));
}

// No public workspace-transfer API exists. Change only this disposable fixture
// to model an administrative transfer, then exercise normal authenticated reads.
export function transferUsageWorkspace(
  backend: BackendContext,
  workspace: string,
  destination: UsageTenant,
) {
  const database = authUsageDatabase(backend);
  try {
    database.exec("PRAGMA busy_timeout=5000; BEGIN IMMEDIATE");
    database
      .prepare("UPDATE workspaces SET org_id=?, owner_id=?, unit_id=? WHERE id=?")
      .run(destination.org.id, destination.owner, destination.unit, workspace);
    database.prepare("DELETE FROM workspace_members WHERE workspace_id=?").run(workspace);
    database.exec("COMMIT");
  } finally {
    database.close();
  }
}

export function retainedOrganizationRows(backend: BackendContext, organization: string) {
  const database = authUsageDatabase(backend);
  try {
    return database
      .prepare("SELECT COUNT(*) AS count FROM usage_scopes WHERE organization_id=?")
      .get(organization) as { count: number };
  } finally {
    database.close();
  }
}

export function retainedOrganizationScopes(backend: BackendContext, organization: string) {
  const database = authUsageDatabase(backend);
  try {
    return (
      database
        .prepare("SELECT scope_id FROM usage_scopes WHERE organization_id=?")
        .all(organization) as { scope_id: string }[]
    ).map((row) => row.scope_id);
  } finally {
    database.close();
  }
}

export function retainedScopeFootprint(backend: BackendContext, scopes: string[]) {
  expect(scopes.length).toBeGreaterThan(0);
  const database = authUsageDatabase(backend);
  try {
    const tables = [
      "usage_scopes",
      "usage_subjects",
      "usage_execution_bindings",
      "session_usage_measurements",
      "session_usage_buckets",
      "usage_observation_items",
      "usage_canonical_facts",
      "usage_projection_applied",
      "usage_projection_contributions",
      "usage_projection_log",
      "usage_daily_summaries",
      "usage_lifetime_summaries",
    ];
    return Object.fromEntries(
      tables.map((table) => {
        const result = database
          .prepare(
            `SELECT COUNT(*) AS count FROM ${table} WHERE scope_id IN (${scopes.map(() => "?").join(",")})`,
          )
          .get(...scopes) as { count: number };
        return [table, Number(result.count)];
      }),
    );
  } finally {
    database.close();
  }
}
