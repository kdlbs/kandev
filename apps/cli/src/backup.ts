import fs from "node:fs";
import os from "node:os";
import path from "node:path";

// Prefix for dev-prod-db automatic snapshots. Distinct from the backend's
// "kandev-*" auto-snapshots and "manual-*" user snapshots so the families
// don't interfere with each other's retention policies.
const BACKUP_PREFIX = "dev-prod-db-";
const BACKUP_SUFFIX = ".db";
const MAX_BACKUPS = 5;

/**
 * Returns true if the given dbPath points to a non-dev database that should
 * be backed up before running dev mode. Dev-isolated databases live under
 * <repo>/.kandev-dev/ and are considered disposable.
 */
export function isProductionDb(dbPath: string): boolean {
  const normalized = path.normalize(dbPath);
  return !normalized.includes(`${path.sep}.kandev-dev${path.sep}`);
}

/**
 * Backs up the database at dbPath to <data-dir>/backups/ before dev mode
 * runs against it. Keeps only the newest MAX_BACKUPS snapshots.
 *
 * Returns the path of the created backup, or null if the DB didn't exist
 * or no backup was needed.
 *
 * The optional `homeDir` parameter is exposed for tests so they can redirect
 * the backup location without mocking os.homedir().
 */
export function backupProductionDb(dbPath: string, homeDir?: string): string | null {
  if (!fs.existsSync(dbPath)) {
    return null;
  }

  const root = homeDir ?? os.homedir();
  const dataDir = path.join(root, ".kandev", "data");
  const backupDir = path.join(dataDir, "backups");
  fs.mkdirSync(backupDir, { recursive: true });

  const ts = new Date().toISOString().replace(/[:.]/g, "");
  const name = `${BACKUP_PREFIX}${ts}${BACKUP_SUFFIX}`;
  const dest = path.join(backupDir, name);

  fs.copyFileSync(dbPath, dest);
  fs.utimesSync(dest, new Date(), new Date());

  pruneBackups(backupDir, MAX_BACKUPS);

  return dest;
}

/**
 * Keeps only the `keep` newest dev-prod-db backup files in `dir`, deleting
 * older ones. Non-matching files are left untouched.
 */
function pruneBackups(dir: string, keep: number): void {
  let entries: fs.Dirent[];
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true });
  } catch {
    return;
  }

  const files = entries
    .filter((e) => e.isFile() && e.name.startsWith(BACKUP_PREFIX) && e.name.endsWith(BACKUP_SUFFIX))
    .map((e) => {
      const fullPath = path.join(dir, e.name);
      try {
        const stat = fs.statSync(fullPath);
        return { path: fullPath, mtime: stat.mtime };
      } catch {
        return null;
      }
    })
    .filter((f): f is { path: string; mtime: Date } => f !== null);

  if (files.length <= keep) {
    return;
  }

  files.sort((a, b) => b.mtime.getTime() - a.mtime.getTime());

  for (const f of files.slice(keep)) {
    try {
      fs.unlinkSync(f.path);
    } catch {
      // Non-critical: don't fail the launch if one old backup can't be removed.
    }
  }
}
