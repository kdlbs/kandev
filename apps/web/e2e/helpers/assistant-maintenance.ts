import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Page } from "@playwright/test";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { ApiClient } from "./api-client";
import type { AssistantBinding } from "../../lib/api/domains/assistant-api";
import { DatabaseSync } from "./node-sqlite";
import { selectExampleAssistant } from "./personal-assistant";

// Synthetic incident projection only. Proposal reads, native resource choices
// and human review use the running backend without intercepted HTTP responses.
function seedExampleImprovement(
  backend: BackendContext,
  binding: AssistantBinding,
  profile: string,
  tasks: string[],
) {
  const db = new DatabaseSync(path.join(backend.tmpDir, "kandev.db"));
  const id = randomUUID(),
    fingerprint = randomUUID(),
    now = new Date().toISOString();
  try {
    db.exec("PRAGMA busy_timeout=5000; BEGIN IMMEDIATE");
    db.prepare(
      `INSERT INTO orchestration_improvements(id,binding_id,workspace_id,fingerprint,profile_id,account_revision,origin,operation,reason,cause,policy_version,state,revision,incident_count,task_count,created_at,updated_at)
      VALUES(?,?,?,?,?,'synthetic-account','native','question','pending_approval','question_request','unknown','proposed',1,3,2,?,?)`,
    ).run(id, binding.id, binding.home_workspace_id, fingerprint, profile, now, now);
    for (let i = 0; i < 3; i++) {
      db.prepare(
        `INSERT INTO orchestration_friction(id,binding_id,workspace_id,task_id,session_id,occurrence_id,profile_id,account_revision,origin,operation,reason,cause,policy_version,fingerprint,outcome,observed_at)
        VALUES(?,?,?,?,'',?,?,'synthetic-account','native','question','pending_approval','question_request','unknown',?,'blocked',?)`,
      ).run(
        randomUUID(),
        binding.id,
        binding.home_workspace_id,
        tasks[i % tasks.length],
        randomUUID(),
        profile,
        fingerprint,
        now,
      );
    }
    db.exec("COMMIT");
  } catch (error) {
    db.exec("ROLLBACK");
    throw error;
  } finally {
    db.close();
  }
  return id;
}

export async function exerciseExampleMaintenance(
  page: Page,
  backend: BackendContext,
  api: ApiClient,
  seed: SeedData,
) {
  const binding = await selectExampleAssistant(page, backend, api, seed);
  const tasks = [];
  for (const title of ["Draft the example guide", "Review the sample checklist"]) {
    tasks.push(
      await api.createTask(seed.workspaceId, title, {
        workflow_id: seed.workflowId,
        workflow_step_id: seed.startStepId,
        repository_ids: [seed.repositoryId],
      }),
    );
  }
  const id = seedExampleImprovement(
    backend,
    binding,
    seed.agentProfileId,
    tasks.map((task) => task.id),
  );
  await page.reload();
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  const card = page.getByTestId("assistant-improvement");
  await expect(card).toHaveCount(1);
  await card.getByRole("button", { name: "Review proposal" }).click();
  await expect(card.getByTestId("maintenance-grant-form")).toBeVisible();
  await expect(card.getByRole("button", { name: "Prepare isolated checkout" })).toBeDisabled();
  await card.locator("summary").filter({ hasText: "Incident evidence" }).click();
  await expect(card.getByRole("link", { name: "Open affected task" })).toHaveCount(3);
  await expect(card.getByText("The policy or classifier version is unknown.")).toBeVisible();
  const review = page.waitForResponse(
    (response) =>
      response.url().endsWith(`/improvements/${id}/review`) &&
      response.request().method() === "POST",
  );
  await card.getByRole("button", { name: "Reject proposal" }).click();
  expect((await review).status()).toBe(200);
  await expect(card.getByText("Rejected", { exact: true })).toBeVisible();
  await expect(card.getByTestId("maintenance-grant-form")).toHaveCount(0);
  const receipt = await page.request.get(
    `${backend.baseUrl}/api/v1/orchestration/assistant/improvements/${id}`,
  );
  expect(receipt.ok()).toBeTruthy();
  expect(await receipt.json()).toMatchObject({
    candidate: { state: "rejected", repair_task_id: "" },
    grant: null,
    review: { state: "rejected" },
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}
