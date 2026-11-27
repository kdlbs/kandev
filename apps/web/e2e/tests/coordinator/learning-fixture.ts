import path from "node:path";
import { DatabaseSync } from "../../helpers/node-sqlite";

export const DREAM_ID = "dream-e2e-1";
export const ITEM_ID = "dream-item-e2e-1";
export const ITEM_TEXT = "Ask before proposing tasks on the Review column";

/** Inserts one clean dream report with two items straight into the e2e database. */
export function seedDream(tmpDir: string, coordinatorId: string): void {
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"));
  try {
    db.exec("PRAGMA busy_timeout = 10000");
    const now = new Date();
    const start = new Date(now.getTime() - 7 * 24 * 3600_000).toISOString();
    const at = new Date(now.getTime() - 6 * 3600_000).toISOString();
    db.prepare(
      `INSERT INTO coordinator_dreams
         (id, coordinator_id, status, reason, window_start, window_end, input_hash,
          turn_ids, considered, model, cost_subcents, started_at, refreshed_at, finished_at)
       VALUES (?, ?, 'clean', '', ?, ?, 'h1', '["t1","t2","t3"]', '["Raise the ceiling"]',
               'mock', 4200, ?, ?, ?)`,
    ).run(DREAM_ID, coordinatorId, start, at, at, at, at);
    const item = db.prepare(
      `INSERT INTO coordinator_dream_items
         (id, dream_id, position, kind, text, target_id, cited_turn_ids, gate)
       VALUES (?, ?, ?, ?, ?, '', '["t1","t2"]', ?)`,
    );
    item.run(ITEM_ID, DREAM_ID, 0, "note_add", ITEM_TEXT, "pass");
    item.run("dream-item-e2e-2", DREAM_ID, 1, "context_diff", "Say what changed.", "thin_evidence");
  } finally {
    db.close();
  }
}
