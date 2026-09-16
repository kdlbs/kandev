import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { DatabaseSync } from "../../helpers/node-sqlite";
import {
  activateRetention,
  analyzeRetention,
  PAYLOAD_TEXT,
  readRetentionMessage,
  resetRetention,
  retentionStatus,
  RETENTION_ROUTE,
  runRetention,
  seedRetentionTask,
  TOOL_COMMAND,
} from "../../helpers/tool-payload-retention";

test.describe("Tool payload retention", () => {
  for (const choice of ["backup", "skip"] as const) {
    test(`analyzes while disabled and cleans old payloads after explicit ${choice}`, async ({
      testPage: page,
      backend,
      apiClient,
      seedData,
    }) => {
      await resetRetention(page);
      const old = await seedRetentionTask(backend.tmpDir, apiClient, seedData, false);
      const recent = await seedRetentionTask(backend.tmpDir, apiClient, seedData, true);
      const original = readRetentionMessage(backend.tmpDir, old.messageId);
      const protectedOriginal = readRetentionMessage(backend.tmpDir, recent.messageId);
      const backupsBefore = await page.request.get("/api/v1/system/backups");
      expect(backupsBefore.ok()).toBe(true);
      const before = (await backupsBefore.json()).snapshots as { name: string }[];
      try {
        await page.goto(RETENTION_ROUTE);
        await expect(page.getByTestId("tool-payload-enabled")).not.toBeChecked();
        const age = await page.getByTestId("tool-payload-age").boundingBox();
        const unit = await page.getByTestId("tool-payload-unit").boundingBox();
        expect(age!.width).toBeLessThanOrEqual(90);
        expect(unit!.width).toBeLessThanOrEqual(140);
        expect(Math.abs(age!.y - unit!.y)).toBeLessThan(2);
        const toggle = await page.getByTestId("tool-payload-enabled").boundingBox();
        const label = await page.getByText("Automatic compaction", { exact: true }).boundingBox();
        expect(label!.x - toggle!.x - toggle!.width).toBeLessThanOrEqual(12);
        await expect(
          page.getByText("Checks every 24 hours while Kandev is running."),
        ).toBeVisible();
        await expect(page.getByTestId("tool-payload-run")).toBeDisabled();
        await analyzeRetention(page);
        await page.getByTestId("tool-payload-retention-card").screenshot({
          path: `/tmp/compaction-desktop-${choice}.png`,
        });
        const analyzed = await retentionStatus(page);
        expect(analyzed.policy.enabled).toBe(false);
        expect(analyzed.last_analysis?.eligible_messages).toBeGreaterThanOrEqual(1);
        expect(analyzed.last_analysis?.payload_bytes).toBeGreaterThan(1024);
        expect(readRetentionMessage(backend.tmpDir, old.messageId)).toEqual(original);
        expect(readRetentionMessage(backend.tmpDir, recent.messageId)).toEqual(protectedOriginal);

        await activateRetention(page, choice);
        await expect(page.getByTestId("tool-payload-age")).toHaveValue("3");
        await expect(page.getByTestId("tool-payload-unit")).toContainText("Months");
        const listing = await page.request.get("/api/v1/system/backups");
        expect(listing.ok()).toBe(true);
        const after = (await listing.json()).snapshots as { name: string }[];
        const added = after.filter(
          (snapshot) => !before.some((item) => item.name === snapshot.name),
        );
        if (choice === "backup") {
          expect(added.length).toBeGreaterThan(0);
          const snapshot = new DatabaseSync(path.join(backend.tmpDir, "backups", added[0].name), {
            readOnly: true,
          });
          try {
            const row = snapshot
              .prepare("SELECT metadata FROM task_session_messages WHERE id=?")
              .get(old.messageId) as { metadata: string };
            expect(JSON.parse(row.metadata)).toEqual(original.metadata);
          } finally {
            snapshot.close();
          }
        } else expect(added).toHaveLength(0);

        await runRetention(page);
        const reduced = readRetentionMessage(backend.tmpDir, old.messageId);
        expect(reduced).toMatchObject({
          id: old.messageId,
          task_session_id: old.sessionId,
          turn_id: old.turnId,
          type: "tool_execute",
          content: TOOL_COMMAND,
          metadata: {
            status: "complete",
            retained_fixture: true,
            tool_call_id: original.metadata.tool_call_id,
            payload_retention: { version: 1 },
            normalized: {
              kind: "shell_exec",
              shell_exec: { command: TOOL_COMMAND, output: { exit_code: 0 } },
            },
          },
        });
        expect(reduced.metadata.normalized.shell_exec.output).not.toHaveProperty("stdout");
        expect(reduced.metadata.normalized.shell_exec.output).not.toHaveProperty("stderr");
        expect(readRetentionMessage(backend.tmpDir, recent.messageId)).toEqual(protectedOriginal);
        await page.goto(`/t/${old.taskId}`);
        await expect(page.getByTestId("tool-execute-command")).toHaveText(TOOL_COMMAND);
        await expect(page.getByText(/^Tool details removed on /)).toBeVisible();
        await expect(page.getByLabel("Command succeeded", { exact: true })).toBeVisible();
        await expect(page.getByText(PAYLOAD_TEXT.trim(), { exact: true })).toHaveCount(0);
        await page.reload();
        await expect(page.getByText(/^Tool details removed on /)).toBeVisible();
        await expect(page.getByTestId("tool-execute-command")).toHaveText(TOOL_COMMAND);
      } finally {
        await resetRetention(page);
        const listing = await page.request.get("/api/v1/system/backups");
        expect(listing.ok()).toBe(true);
        const snapshots = (await listing.json()).snapshots as { name: string }[];
        for (const snapshot of snapshots.filter(
          (item) => !before.some((saved) => saved.name === item.name),
        )) {
          const removed = await page.request.delete(
            `/api/v1/system/backups/${encodeURIComponent(snapshot.name)}`,
          );
          expect(removed.ok(), await removed.text()).toBe(true);
        }
        await apiClient.deleteTask(old.taskId);
        await apiClient.deleteTask(recent.taskId);
      }
    });
  }
});
