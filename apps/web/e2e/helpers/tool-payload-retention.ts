import path from "node:path";
import { randomUUID } from "node:crypto";
import { expect, type Locator, type Page } from "@playwright/test";
import { DatabaseSync } from "./node-sqlite";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { ToolPayloadRetentionStatus } from "../../lib/types/tool-payload-retention";

export const RETENTION_API = "/api/v1/system/database/tool-payload-retention";
export const RETENTION_ROUTE = "/settings/system/data-storage";
export const PAYLOAD_TEXT = "retention fixture output\n".repeat(1024);
export const TOOL_COMMAND = "printf retention-fixture";

function fixtureDatabase(tmpDir: string) {
  if (!/^kandev-e2e-\d+-/.test(path.basename(tmpDir))) {
    throw new Error("retention seeding requires a disposable E2E worker directory");
  }
  return path.join(tmpDir, "kandev.db");
}

type RetentionFixture = { taskId: string; sessionId: string; messageId: string; turnId: string };

/** Only the worker's disposable database is opened, never an operator database. */
export async function seedRetentionTask(
  tmpDir: string,
  api: ApiClient,
  seed: SeedData,
  recent: boolean,
): Promise<RetentionFixture> {
  const task = await api.createTask(
    seed.workspaceId,
    recent ? "Recent retention task" : "Old retention task",
    {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    },
  );
  const fixture = {
    taskId: task.id,
    sessionId: randomUUID(),
    messageId: randomUUID(),
    turnId: randomUUID(),
  };
  const old = new Date(Date.now() - 400 * 86400000).toISOString();
  const activity = recent ? new Date().toISOString() : old;
  const metadata = JSON.stringify({
    tool_call_id: randomUUID(),
    status: "complete",
    retained_fixture: true,
    normalized: {
      kind: "shell_exec",
      shell_exec: {
        command: TOOL_COMMAND,
        work_dir: "/tmp",
        output: { stdout: PAYLOAD_TEXT, stderr: "", exit_code: 0 },
      },
    },
  });
  const db = new DatabaseSync(fixtureDatabase(tmpDir));
  try {
    db.exec("PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; BEGIN IMMEDIATE");
    db.prepare("UPDATE tasks SET state='COMPLETED', created_at=?, updated_at=? WHERE id=?").run(
      old,
      old,
      task.id,
    );
    db.prepare(
      `INSERT INTO task_sessions (id,task_id,state,started_at,completed_at,updated_at,is_primary)
      VALUES (?,?,'COMPLETED',?,?,?,1)`,
    ).run(fixture.sessionId, task.id, old, old, old);
    db.prepare(
      `INSERT INTO task_session_turns (id,task_session_id,task_id,started_at,completed_at,created_at,updated_at)
      VALUES (?,?,?,?,?,?,?)`,
    ).run(fixture.turnId, fixture.sessionId, task.id, old, old, old, old);
    db.prepare(
      `INSERT INTO task_session_messages (id,task_session_id,task_id,turn_id,author_type,content,type,metadata,created_at,updated_at)
      VALUES (?,?,?,?,'agent',?,'tool_execute',?,?,?)`,
    ).run(
      fixture.messageId,
      fixture.sessionId,
      task.id,
      fixture.turnId,
      TOOL_COMMAND,
      metadata,
      old,
      activity,
    );
    db.exec("COMMIT");
  } catch (error) {
    db.exec("ROLLBACK");
    throw error;
  } finally {
    db.close();
  }
  return fixture;
}

export function readRetentionMessage(tmpDir: string, messageId: string) {
  const db = new DatabaseSync(fixtureDatabase(tmpDir), { readOnly: true });
  try {
    const row = db
      .prepare(
        "SELECT id, task_session_id, turn_id, type, content, metadata FROM task_session_messages WHERE id=?",
      )
      .get(messageId) as {
      id: string;
      task_session_id: string;
      turn_id: string;
      type: string;
      content: string;
      metadata: string;
    };
    expect(row).toBeTruthy();
    return { ...row, metadata: JSON.parse(row.metadata) };
  } finally {
    db.close();
  }
}

export async function retentionStatus(page: Page): Promise<ToolPayloadRetentionStatus> {
  const response = await page.request.get(RETENTION_API);
  expect(response.ok(), await response.text()).toBe(true);
  return response.json();
}

export async function resetRetention(page: Page) {
  const { policy } = await retentionStatus(page);
  const response = await page.request.put(RETENTION_API, {
    data: { ...policy, enabled: false, age: { value: 3, unit: "months" } },
  });
  expect(response.ok(), await response.text()).toBe(true);
}

export async function press(locator: Locator, touch = false) {
  if (touch) await locator.tap();
  else await locator.click();
}

export async function analyzeRetention(page: Page, touch = false) {
  const accepted = page.waitForResponse(
    (r) =>
      new URL(r.url()).pathname === `${RETENTION_API}/analyze` && r.request().method() === "POST",
  );
  await press(page.getByTestId("tool-payload-analyze"), touch);
  const response = await accepted;
  expect(response.status()).toBe(202);
  const { operation_id: id } = await response.json();
  await expect
    .poll(
      async () => {
        const status = await retentionStatus(page);
        return status.last_analysis?.id === id ? status.last_analysis.state : "pending";
      },
      { timeout: 30_000 },
    )
    .toBe("succeeded");
  await expect(page.getByTestId("tool-payload-estimate")).toContainText("Completed");
}

export async function activateRetention(page: Page, choice: "backup" | "skip", touch = false) {
  await press(page.locator('label[for="tool-payload-enabled"]'), touch);
  await expect(page.getByRole("button", { name: "Save changes", exact: true })).toBeDisabled();
  await expect(page.getByTestId("tool-payload-backup")).not.toBeChecked();
  await expect(page.getByTestId("tool-payload-skip")).not.toBeChecked();
  const choiceTarget = page.getByTestId(`tool-payload-${choice}`).locator("..");
  if (touch) await expectTouchTarget(choiceTarget);
  await press(choiceTarget, touch);
  if (touch)
    await expectTouchTarget(page.getByRole("button", { name: "Save changes", exact: true }));
  const saved = page.waitForResponse(
    (r) => new URL(r.url()).pathname === RETENTION_API && r.request().method() === "PUT",
  );
  await press(page.getByRole("button", { name: "Save changes", exact: true }), touch);
  const response = await saved;
  expect(response.ok(), await response.text()).toBe(true);
  expect(response.request().postDataJSON()).toMatchObject({ enabled: true, backup_choice: choice });
  await expect
    .poll(async () => (await retentionStatus(page)).policy.enabled, { timeout: 30_000 })
    .toBe(true);
  await page.reload();
  await expect(page.getByTestId("tool-payload-enabled")).toBeChecked();
}

export async function runRetention(page: Page, touch = false) {
  const accepted = page.waitForResponse(
    (r) => new URL(r.url()).pathname === `${RETENTION_API}/run` && r.request().method() === "POST",
  );
  await expect(page.getByTestId("tool-payload-run")).toBeEnabled();
  await press(page.getByTestId("tool-payload-run"), touch);
  const response = await accepted;
  expect(response.status()).toBe(202);
  const { operation_id: id } = await response.json();
  await expect
    .poll(
      async () => {
        const status = await retentionStatus(page);
        return status.last_run?.id === id ? status.last_run.state : "pending";
      },
      { timeout: 30_000 },
    )
    .toBe("succeeded");
  await expect(page.getByTestId("tool-payload-last-run")).toContainText("Completed");
}

export async function expectTouchTarget(locator: Locator) {
  await locator.scrollIntoViewIfNeeded();
  const bounds = await locator.boundingBox();
  expect(bounds).not.toBeNull();
  expect(bounds!.height).toBeGreaterThanOrEqual(44);
  expect(bounds!.width).toBeGreaterThanOrEqual(44);
  expect(
    await locator.evaluate((el) => {
      const box = el.getBoundingClientRect();
      const hit = document.elementFromPoint(box.x + box.width / 2, box.y + box.height / 2);
      return box.left >= 0 && box.right <= innerWidth && (hit === el || el.contains(hit));
    }),
  ).toBe(true);
}
