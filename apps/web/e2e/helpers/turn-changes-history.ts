import { DatabaseSync } from "node:sqlite";

export function markTurnChangeLegacy(dbPath: string, changeSetID: string) {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    db.prepare(
      "UPDATE turn_repository_changes SET comparison_basis = 'legacy_raw' WHERE change_set_id = ?",
    ).run(changeSetID);
  } finally {
    db.close();
  }
}

export type TurnFileLink = { variant: string; contentID: string };

export function pauseTurnFileContent(
  dbPath: string,
  changeSetID: string,
  path: string,
): { fileID: string; links: TurnFileLink[] } {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const file = db
      .prepare(
        `SELECT f.id, f.repository_change_id AS repositoryChangeID
         FROM turn_file_changes f
         JOIN turn_repository_changes r ON r.id = f.repository_change_id
         WHERE r.change_set_id = ? AND f.path = ?`,
      )
      .get(changeSetID, path) as { id: string; repositoryChangeID: string } | undefined;
    if (!file) throw new Error(`turn file not found: ${path}`);
    const links = db
      .prepare(
        "SELECT variant, content_id AS contentID FROM turn_change_content_links WHERE file_change_id = ?",
      )
      .all(file.id) as TurnFileLink[];
    db.prepare("DELETE FROM turn_change_content_links WHERE file_change_id = ?").run(file.id);
    db.prepare(
      `UPDATE turn_file_changes
       SET content_availability = 'pending', content_reason = '',
           canonical_content_id = '', filtered_content_id = '', old_content_id = '', new_content_id = ''
       WHERE id = ?`,
    ).run(file.id);
    db.prepare(
      `UPDATE turn_repository_changes
       SET export_state = 'queued', export_reason = '', content_complete = 0,
           export_next_attempt_at = ?
       WHERE id = ?`,
    ).run(new Date(Date.now() + 60 * 60 * 1000).toISOString(), file.repositoryChangeID);
    db.prepare(
      "UPDATE turn_change_sets SET content_complete = 0, complete = 0, revision = revision + 1 WHERE id = ?",
    ).run(changeSetID);
    return { fileID: file.id, repositoryChangeID: file.repositoryChangeID, links };
  } finally {
    db.close();
  }
}

export function resumeTurnFileContent(
  dbPath: string,
  changeSetID: string,
  fileID: string,
  repositoryChangeID: string,
  links: TurnFileLink[],
) {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const insert = db.prepare(
      "INSERT OR REPLACE INTO turn_change_content_links (file_change_id, variant, content_id, created_at) VALUES (?, ?, ?, ?)",
    );
    for (const link of links) {
      insert.run(fileID, link.variant, link.contentID, new Date().toISOString());
    }
    db.prepare(
      `UPDATE turn_file_changes
       SET content_availability = 'ready', content_truncated = 0
       WHERE id = ?`,
    ).run(fileID);
    db.prepare(
      `UPDATE turn_repository_changes
       SET export_state = 'complete', content_complete = 1, export_next_attempt_at = NULL
       WHERE id = ?`,
    ).run(repositoryChangeID);
    db.prepare(
      "UPDATE turn_change_sets SET content_complete = 1, complete = summary_complete, revision = revision + 1 WHERE id = ?",
    ).run(changeSetID);
  } finally {
    db.close();
  }
}

export function setTurnChangeOverlaps(dbPath: string, changeSetID: string, overlaps: unknown[]) {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    db.prepare(
      "UPDATE turn_change_sets SET overlap_intervals_json = ?, revision = revision + 1 WHERE id = ?",
    ).run(JSON.stringify(overlaps), changeSetID);
  } finally {
    db.close();
  }
}

export function addUnavailableRepositoryRoot(dbPath: string, changeSetID: string) {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const now = new Date().toISOString();
    db.prepare(
      `INSERT INTO turn_repository_changes
       (id, change_set_id, checkout_id, display_name, availability, reason, comparison_basis,
        comparison_reason, created_at, updated_at)
       VALUES (?, ?, ?, ?, 'unavailable', 'history_conflict', 'history_unavailable',
               'history_conflict', ?, ?)`,
    ).run(
      `${changeSetID}-unavailable-root`,
      changeSetID,
      `${changeSetID}-unavailable-checkout`,
      "legacy-sibling",
      now,
      now,
    );
    db.prepare(
      `UPDATE turn_change_sets
       SET repository_count = 2, summary_complete = 0, complete = 0, revision = revision + 1
       WHERE id = ?`,
    ).run(changeSetID);
  } finally {
    db.close();
  }
}
