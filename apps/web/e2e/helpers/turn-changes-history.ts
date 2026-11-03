import { DatabaseSync } from "node:sqlite";
import { dwell } from "./causal-waits";

const CONTENT_SETTLE_TIMEOUT_MS = 60_000;
const CONTENT_SETTLE_POLL_MS = 100;
const PAUSE_EXPORT_DEFERRAL_MS = 60 * 60 * 1000;

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

export type TurnFileLink = { variant: string; contentID: string; createdAt: string };

type TurnFileSnapshot = {
  contentAvailability: string;
  contentReason: string;
  contentTruncated: number;
  canonicalContentID: string;
  filteredContentID: string;
  oldContentID: string;
  newContentID: string;
};

type TurnRepositorySnapshot = {
  contentSource: string;
  sourceGitDir: string;
  preservationState: string;
  preservationReason: string;
  preservationAttempts: number;
  preservationBytes: number;
  preservationReceipt: string;
  contentComplete: number;
  exportState: string;
  exportReason: string;
  exportClaimOwner: string;
  exportClaimToken: number;
  exportClaimExpiresAt: string | null;
  exportNextAttemptAt: string | null;
};

type TurnChangeSetSnapshot = {
  contentComplete: number;
  complete: number;
};

export type TurnFileContentSnapshot = {
  file: TurnFileSnapshot;
  repository: TurnRepositorySnapshot;
  changeSet: TurnChangeSetSnapshot;
  links: TurnFileLink[];
};

export type TurnFileContentPause = {
  fileID: string;
  repositoryChangeID: string;
  snapshot: TurnFileContentSnapshot;
};

function readTurnFile(db: DatabaseSync, changeSetID: string, path: string) {
  return db
    .prepare(
      `SELECT f.id, f.repository_change_id AS repositoryChangeID,
              f.content_availability AS contentAvailability, f.content_reason AS contentReason,
              f.content_truncated AS contentTruncated,
              f.canonical_content_id AS canonicalContentID,
              f.filtered_content_id AS filteredContentID,
              f.old_content_id AS oldContentID, f.new_content_id AS newContentID
       FROM turn_file_changes f
       JOIN turn_repository_changes r ON r.id = f.repository_change_id
       WHERE r.change_set_id = ? AND f.path = ?`,
    )
    .get(changeSetID, path) as
    | ({ id: string; repositoryChangeID: string } & TurnFileSnapshot)
    | undefined;
}

type TurnFileContentObservation = { settled: boolean; description: string };

type RepositorySettleRow = {
  contentComplete: number;
  preservationState: string;
  exportState: string;
};

type ChangeSetSettleRow = {
  contentComplete: number;
  complete: number;
  summaryComplete: number;
};

function readRepositorySettleRow(
  db: DatabaseSync,
  repositoryChangeID: string,
): RepositorySettleRow | undefined {
  return db
    .prepare(
      `SELECT content_complete AS contentComplete,
              preservation_state AS preservationState, export_state AS exportState
       FROM turn_repository_changes WHERE id = ?`,
    )
    .get(repositoryChangeID) as RepositorySettleRow | undefined;
}

function readChangeSetSettleRow(
  db: DatabaseSync,
  changeSetID: string,
): ChangeSetSettleRow | undefined {
  return db
    .prepare(
      `SELECT content_complete AS contentComplete, complete,
              summary_complete AS summaryComplete
       FROM turn_change_sets WHERE id = ?`,
    )
    .get(changeSetID) as ChangeSetSettleRow | undefined;
}

function countPendingRepositories(db: DatabaseSync, changeSetID: string): number {
  const row = db
    .prepare(
      `SELECT COUNT(*) AS count FROM turn_repository_changes
       WHERE change_set_id = ? AND preservation_state = 'pending'`,
    )
    .get(changeSetID) as { count: number };
  return row.count;
}

function countActiveLeases(db: DatabaseSync, changeSetID: string): number {
  const row = db
    .prepare(
      `SELECT COUNT(*) AS count FROM turn_change_content_leases
       WHERE change_set_id = ? AND julianday(expires_at) > julianday('now')`,
    )
    .get(changeSetID) as { count: number };
  return row.count;
}

function isContentSettled(
  repository: RepositorySettleRow | undefined,
  changeSet: ChangeSetSettleRow | undefined,
  pendingRepositories: number,
  activeLeases: number,
): boolean {
  if (!repository || !changeSet) return false;
  if (!repository.contentComplete) return false;
  if (changeSet.contentComplete !== 1) return false;
  if (changeSet.complete !== changeSet.summaryComplete) return false;
  const preservationKnown =
    repository.preservationState === "" || repository.preservationState === "ready";
  const readable =
    repository.preservationState === "ready" ||
    repository.exportState === "complete" ||
    repository.exportState === "unavailable";
  return preservationKnown && readable && pendingRepositories === 0 && activeLeases === 0;
}

function describeSettleState(
  repository: RepositorySettleRow | undefined,
  changeSet: ChangeSetSettleRow | undefined,
  pendingRepositories: number,
  activeLeases: number,
): string {
  const repositoryText = repository
    ? `content_complete=${repository.contentComplete} ` +
      `preservation_state=${repository.preservationState || "''"} ` +
      `export_state=${repository.exportState || "''"}`
    : "repository=missing";
  const changeSetText = changeSet
    ? `change_set_content_complete=${changeSet.contentComplete} ` +
      `change_set_complete=${changeSet.complete} ` +
      `change_set_summary_complete=${changeSet.summaryComplete}`
    : "change_set=missing";
  return (
    `${repositoryText} pending_repositories=${pendingRepositories} ` +
    `active_leases=${activeLeases} ${changeSetText}`
  );
}

function observeTurnFileContent(
  db: DatabaseSync,
  changeSetID: string,
  path: string,
): TurnFileContentObservation {
  const file = readTurnFile(db, changeSetID, path);
  if (!file) return { settled: false, description: `turn file not found: ${path}` };
  const repository = readRepositorySettleRow(db, file.repositoryChangeID);
  const changeSet = readChangeSetSettleRow(db, changeSetID);
  const pendingRepositories = countPendingRepositories(db, changeSetID);
  const activeLeases = countActiveLeases(db, changeSetID);
  return {
    settled: isContentSettled(repository, changeSet, pendingRepositories, activeLeases),
    description: describeSettleState(repository, changeSet, pendingRepositories, activeLeases),
  };
}

/**
 * Wait until the turn's content owner has settled this file. A finalized
 * terminal (terminal_at) does not mean preservation finished: the coordinator
 * runs content processing afterwards, so a pause taken on terminal_at alone can
 * race the producer or an in-flight reader lease.
 */
export async function waitForTurnFileContentSettled(
  dbPath: string,
  changeSetID: string,
  path: string,
  timeoutMs = CONTENT_SETTLE_TIMEOUT_MS,
) {
  const deadline = Date.now() + timeoutMs;
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    for (;;) {
      const observation = observeTurnFileContent(db, changeSetID, path);
      if (observation.settled) return;
      if (Date.now() >= deadline) {
        throw new Error(
          `turn file content did not settle within ${timeoutMs}ms ` +
            `(${observation.description}): ${path}`,
        );
      }
      await dwell(
        CONTENT_SETTLE_POLL_MS,
        "poll-interval",
        "turn content owner publishes no event the test can subscribe to",
      );
    }
  } finally {
    db.close();
  }
}

export function pauseTurnFileContent(
  dbPath: string,
  changeSetID: string,
  path: string,
): TurnFileContentPause {
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    const file = readTurnFile(db, changeSetID, path);
    if (!file) throw new Error(`turn file not found: ${path}`);
    const repository = db
      .prepare(
        `SELECT content_source AS contentSource, source_git_dir AS sourceGitDir,
                preservation_state AS preservationState, preservation_reason AS preservationReason,
                preservation_attempts AS preservationAttempts,
                preservation_bytes AS preservationBytes,
                preservation_receipt AS preservationReceipt,
                content_complete AS contentComplete, export_state AS exportState,
                export_reason AS exportReason, export_claim_owner AS exportClaimOwner,
                export_claim_token AS exportClaimToken,
                export_claim_expires_at AS exportClaimExpiresAt,
                export_next_attempt_at AS exportNextAttemptAt
         FROM turn_repository_changes WHERE id = ?`,
      )
      .get(file.repositoryChangeID) as TurnRepositorySnapshot | undefined;
    if (!repository) throw new Error(`turn repository not found: ${file.repositoryChangeID}`);
    const changeSet = db
      .prepare(
        "SELECT content_complete AS contentComplete, complete FROM turn_change_sets WHERE id = ?",
      )
      .get(changeSetID) as TurnChangeSetSnapshot | undefined;
    if (!changeSet) throw new Error(`turn change set not found: ${changeSetID}`);
    const links = db
      .prepare(
        "SELECT variant, content_id AS contentID, created_at AS createdAt FROM turn_change_content_links WHERE file_change_id = ?",
      )
      .all(file.id) as TurnFileLink[];

    db.prepare("DELETE FROM turn_change_content_links WHERE file_change_id = ?").run(file.id);
    db.prepare(
      `UPDATE turn_file_changes
       SET content_availability = 'pending', content_reason = '', content_truncated = 0,
           canonical_content_id = '', filtered_content_id = '', old_content_id = '', new_content_id = ''
       WHERE id = ?`,
    ).run(file.id);
    db.prepare(
      `UPDATE turn_repository_changes
       SET content_source = '', source_git_dir = '', preservation_state = '',
           preservation_reason = '', preservation_receipt = '', content_complete = 0,
           export_state = 'queued', export_reason = '', export_claim_owner = '',
           export_claim_token = 0, export_claim_expires_at = NULL, export_next_attempt_at = ?
       WHERE id = ?`,
    ).run(new Date(Date.now() + PAUSE_EXPORT_DEFERRAL_MS).toISOString(), file.repositoryChangeID);
    db.prepare(
      "UPDATE turn_change_sets SET content_complete = 0, complete = 0, revision = revision + 1 WHERE id = ?",
    ).run(changeSetID);
    return {
      fileID: file.id,
      repositoryChangeID: file.repositoryChangeID,
      snapshot: {
        file: {
          contentAvailability: file.contentAvailability,
          contentReason: file.contentReason,
          contentTruncated: file.contentTruncated,
          canonicalContentID: file.canonicalContentID,
          filteredContentID: file.filteredContentID,
          oldContentID: file.oldContentID,
          newContentID: file.newContentID,
        },
        repository,
        changeSet,
        links,
      },
    };
  } finally {
    db.close();
  }
}

export function resumeTurnFileContent(
  dbPath: string,
  changeSetID: string,
  paused: TurnFileContentPause,
) {
  const { fileID, repositoryChangeID, snapshot } = paused;
  const db = new DatabaseSync(dbPath);
  try {
    db.exec("PRAGMA busy_timeout = 5000");
    db.prepare("DELETE FROM turn_change_content_links WHERE file_change_id = ?").run(fileID);
    const insert = db.prepare(
      "INSERT OR REPLACE INTO turn_change_content_links (file_change_id, variant, content_id, created_at) VALUES (?, ?, ?, ?)",
    );
    for (const link of snapshot.links) {
      insert.run(fileID, link.variant, link.contentID, link.createdAt);
    }
    db.prepare(
      `UPDATE turn_file_changes
       SET content_availability = ?, content_reason = ?, content_truncated = ?,
           canonical_content_id = ?, filtered_content_id = ?, old_content_id = ?, new_content_id = ?
       WHERE id = ?`,
    ).run(
      snapshot.file.contentAvailability,
      snapshot.file.contentReason,
      snapshot.file.contentTruncated,
      snapshot.file.canonicalContentID,
      snapshot.file.filteredContentID,
      snapshot.file.oldContentID,
      snapshot.file.newContentID,
      fileID,
    );
    db.prepare(
      `UPDATE turn_repository_changes
       SET content_source = ?, source_git_dir = ?, preservation_state = ?,
           preservation_reason = ?, preservation_attempts = ?, preservation_bytes = ?,
           preservation_receipt = ?, content_complete = ?, export_state = ?, export_reason = ?,
           export_claim_owner = ?, export_claim_token = ?, export_claim_expires_at = ?,
           export_next_attempt_at = ?
       WHERE id = ?`,
    ).run(
      snapshot.repository.contentSource,
      snapshot.repository.sourceGitDir,
      snapshot.repository.preservationState,
      snapshot.repository.preservationReason,
      snapshot.repository.preservationAttempts,
      snapshot.repository.preservationBytes,
      snapshot.repository.preservationReceipt,
      snapshot.repository.contentComplete,
      snapshot.repository.exportState,
      snapshot.repository.exportReason,
      snapshot.repository.exportClaimOwner,
      snapshot.repository.exportClaimToken,
      snapshot.repository.exportClaimExpiresAt,
      snapshot.repository.exportNextAttemptAt,
      repositoryChangeID,
    );
    db.prepare(
      `UPDATE turn_change_sets
       SET content_complete = ?, complete = ?, revision = revision + 1
       WHERE id = ?`,
    ).run(snapshot.changeSet.contentComplete, snapshot.changeSet.complete, changeSetID);
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
