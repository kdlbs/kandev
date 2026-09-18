import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { AssistantBinding } from "../../lib/api/domains/assistant-api";
import { DatabaseSync } from "./node-sqlite";
import type { PrAssetCapture } from "./pr-asset-capture";
export const ASSISTANT_ENV = {
  KANDEV_FEATURES_ORCHESTRATION: "true",
  KANDEV_FEATURES_PERSONAL_ASSISTANT: "true",
  KANDEV_FEATURES_OFFICE: "false",
};
export async function selectExampleAssistant(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
) {
  const base = `${backend.baseUrl}/api/v1/orchestration`;
  const { executors } = await api.listExecutors();
  const local = executors.find((row) => row.type === "local")?.profiles?.[0];
  expect(local).toBeTruthy();
  const roleResponse = await page.request.post(`${base}/roles`, {
    data: { name: "Example assistant", instructions: "Help organize sample documentation tasks." },
  });
  expect(roleResponse.ok()).toBeTruthy();
  const role = await roleResponse.json();
  const created = await page.request.post(`${base}/workspaces/${seed.workspaceId}/orchestrators`, {
    data: {
      role_id: role.id,
      profile_id: seed.agentProfileId,
      executor_preference: JSON.stringify({ executor_profile_id: local!.id }),
      context: "Use synthetic example tasks.",
    },
  });
  expect(created.ok()).toBeTruthy();
  const orchestrator = await created.json();
  await page.goto("/assistant");
  await expect(page.getByTestId("assistant-setup")).toBeVisible();
  await page.getByTestId("assistant-workspace").click();
  await page.getByRole("option", { name: "E2E Workspace", exact: true }).click();
  await page.getByTestId("assistant-selector").click();
  await page.getByRole("option", { name: role.name, exact: true }).click();
  await page.getByRole("button", { name: "Use this assistant" }).click();
  await expect(page.getByTestId("assistant-page")).toBeVisible();
  const response = await page.request.get(`${base}/assistant`);
  expect(response.ok()).toBeTruthy();
  const binding = (await response.json()) as AssistantBinding;
  expect(binding.orchestrator_id).toBe(orchestrator.id);
  return binding;
}
// Populate only the owned worker database. Native questions and their resolution
// still come from a running mock worker and the production APIs, never HTTP stubs.
export function seedExampleObjective(
  backend: BackendContext,
  binding: AssistantBinding,
  taskId: string,
) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  const source = randomUUID();
  const objective = randomUUID();
  try {
    db.exec("PRAGMA busy_timeout=5000; BEGIN IMMEDIATE");
    db.prepare(
      "INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES (?,?,'user',?,?,'user',CURRENT_TIMESTAMP)",
    ).run(
      source,
      binding.conversation_id,
      binding.owner_user_id,
      "Please use short headings for the example guide.",
    );
    db.prepare(
      "INSERT INTO orchestration_objectives(id,binding_id,workspace_id,source_comment_id,title,mode,status,revision,acceptance_revision,intent_revision,acceptance_json,evidence_json,created_at,updated_at) VALUES (?,?,?,?,'Prepare the example guide','design','active',1,1,?,?,'[]',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)",
    ).run(
      objective,
      binding.id,
      binding.home_workspace_id,
      source,
      binding.intent_revision,
      JSON.stringify([
        { id: "heading", description: "Choose a clear heading for the sample guide." },
      ]),
    );
    db.prepare(
      "INSERT INTO orchestration_objective_tasks(objective_id,task_id,session_id,role,context_ref,operation_id) VALUES (?,?,'','worker','','example-link')",
    ).run(objective, taskId);
    db.exec("COMMIT");
  } catch (error) {
    db.exec("ROLLBACK");
    throw error;
  } finally {
    db.close();
  }
  return source;
}
export async function exerciseExampleAssistant(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
  { mobile, capture }: { mobile: boolean; capture?: PrAssetCapture },
) {
  const binding = await selectExampleAssistant(page, backend, api, seed);
  // The generic mock provider is deliberately unsupported by the restricted
  // assistant. This flow verifies human controls; task 11 qualifies the provider.
  await expect(
    page.getByText("This execution profile cannot currently run the assistant.", { exact: false }),
  ).toBeVisible();
  const task = await api.createTask(seed.workspaceId, "Choose a sample guide heading", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
    repository_ids: [seed.repositoryId],
  });
  const source = seedExampleObjective(backend, binding, task.id);
  const session = await api.launchSession({
    task_id: task.id,
    agent_profile_id: seed.agentProfileId,
    workflow_step_id: seed.startStepId,
    prompt:
      'e2e:mcp:kandev:ask_user_question_kandev({"questions":[{"id":"heading","prompt":"Which heading should the sample guide use?","options":[{"label":"Quick start","description":"A concise example"},{"label":"Getting started","description":"A longer example"}]}]})',
  });
  const attention = `${backend.baseUrl}/api/v1/orchestration/assistant/attention`;
  await expect
    .poll(
      async () => {
        const data = await (await page.request.get(attention)).json();
        return data.entries?.some(
          (row: { kind: string; state: string }) =>
            row.kind === "question" && row.state === "pending",
        );
      },
      { timeout: 60000 },
    )
    .toBe(true);
  await page.reload();
  if (mobile) await page.getByRole("tab", { name: "Attention", exact: true }).click();
  await page.getByRole("button", { name: "Review and respond", exact: true }).click();
  await expect(page.getByTestId("assistant-attention-card")).toContainText(
    "Which heading should the sample guide use?",
  );
  await capture?.screenshot(mobile ? "phone-attention" : "desktop-attention", {
    caption: "A generic worker question resolved through the Assistant's native input controls.",
  });
  if (!mobile) await capture?.startRecording("assistant-generic-walkthrough");
  await page.getByTestId("clarification-option").filter({ hasText: "Quick start" }).click();
  await expect
    .poll(
      async () => {
        const data = await api.listSessionMessages(session.session_id);
        return data.messages.find((row) => row.type === "clarification_request")?.metadata?.status;
      },
      { timeout: 30000 },
    )
    .toBe("answered");
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  await expect(page.getByText("Prepare the example guide", { exact: true })).toBeVisible();
  const memory = page
    .locator("details")
    .filter({ has: page.locator("summary", { hasText: /^Memory$/ }) });
  await memory.locator("summary").click();
  await memory.getByRole("button", { name: "Add memory" }).click();
  await memory.getByLabel("Name", { exact: true }).fill("Example headings");
  await memory.getByLabel("What should be remembered").fill("Use short headings in sample guides.");
  await page.getByTestId("assistant-memory-source").click();
  await page
    .getByRole("option", { name: "Please use short headings for the example guide.", exact: true })
    .click();
  await memory.getByLabel("Use this as confirmed context").click();
  await memory.getByRole("button", { name: "Save", exact: true }).click();
  await expect(memory.getByTestId("assistant-memory-row")).toContainText(
    "Use short headings in sample guides.",
  );
  await memory.getByRole("button", { name: "Original instruction" }).click();
  await expect(memory.locator("blockquote")).toHaveText(
    "Please use short headings for the example guide.",
  );
  await capture?.screenshot(mobile ? "phone-memory" : "desktop-memory", {
    caption: "A confirmed example preference with its original generic instruction.",
  });
  if (!mobile) {
    await capture?.stopRecording({
      caption: "Answer a generic worker question and save a confirmed example preference.",
    });
  }
  const rows = await (
    await page.request.get(`${backend.baseUrl}/api/v1/orchestration/assistant/memory`)
  ).json();
  expect(rows.memory[0].source_comment_id).toBe(source);
  await memory.getByRole("button", { name: "Forget", exact: true }).click();
  await expect(memory.getByTestId("assistant-memory-row")).toHaveCount(0);
  await page.getByRole("button", { name: "Pause assistant", exact: true }).click();
  await expect(
    page.getByText("Assistant paused. Existing workers keep running.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Resume assistant", exact: true }).click();
  await expect(page.getByRole("button", { name: "Pause assistant", exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
  const after = await (
    await page.request.get(`${backend.baseUrl}/api/v1/orchestration/assistant`)
  ).json();
  expect(after.conversation_id).toBe(binding.conversation_id);
  expect((await api.listTaskSessions(binding.conversation_id)).sessions).toEqual([]);
}
