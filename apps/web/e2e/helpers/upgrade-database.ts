import { execFileSync } from "node:child_process";

export function postgresSQL(database: string, sql: string) {
  if (!/^(postgres|upgrade_[a-f0-9]+)$/.test(database))
    throw new Error("unowned PostgreSQL database");
  const service = new URL(process.env.KANDEV_UPGRADE_POSTGRES_URL!);
  const container = process.env.KANDEV_UPGRADE_POSTGRES_CONTAINER;
  const args = [
    "-X",
    "-v",
    "ON_ERROR_STOP=1",
    "-At",
    "-d",
    database,
    "-U",
    decodeURIComponent(service.username),
    "-c",
    sql,
  ];
  return container
    ? execFileSync("docker", ["exec", container, "psql", ...args], { encoding: "utf8" }).trim()
    : execFileSync("psql", ["-h", service.hostname, "-p", service.port || "5432", ...args], {
        encoding: "utf8",
        env: { ...process.env, PGPASSWORD: decodeURIComponent(service.password) },
      }).trim();
}
export function prepareUpgradeDatabase(name: string) {
  postgresSQL("postgres", `CREATE DATABASE "${name}";`);
  const url = new URL(process.env.KANDEV_UPGRADE_POSTGRES_URL!);
  return {
    KANDEV_DATABASE_DRIVER: "postgres",
    KANDEV_DATABASE_HOST: url.hostname,
    KANDEV_DATABASE_PORT: url.port || "5432",
    KANDEV_DATABASE_USER: decodeURIComponent(url.username),
    KANDEV_DATABASE_PASSWORD: decodeURIComponent(url.password),
    KANDEV_DATABASE_DBNAME: name,
    KANDEV_DATABASE_SSLMODE: "disable",
  };
}

import fs from "node:fs";
import path from "node:path";
import { expect } from "@playwright/test";
import { DatabaseSync } from "./node-sqlite";
import type { UpgradeBackend } from "../fixtures/upgrade-backend";
import type { UpgradeSeed } from "./upgrade-seed";

const relationshipChecks = [
  "SELECT count(*) AS n FROM tasks c LEFT JOIN workspaces p ON p.id=c.workspace_id WHERE p.id IS NULL",
  "SELECT count(*) AS n FROM task_sessions c LEFT JOIN tasks p ON p.id=c.task_id WHERE p.id IS NULL",
  "SELECT count(*) AS n FROM task_plans c LEFT JOIN tasks p ON p.id=c.task_id WHERE p.id IS NULL",
  "SELECT count(*) AS n FROM task_session_messages c LEFT JOIN task_sessions p ON p.id=c.task_session_id WHERE p.id IS NULL",
];
export function checkUpgradeDatabase(
  backend: UpgradeBackend,
  seed: UpgradeSeed,
  checkBackup: boolean,
) {
  if (backend.engine === "postgres") {
    for (const query of relationshipChecks)
      expect(postgresSQL(backend.postgresName, query)).toBe("0");
    const invalid = postgresSQL(
      backend.postgresName,
      "SELECT count(*) FROM pg_constraint WHERE contype IN ('f','c') AND NOT convalidated",
    );
    expect(invalid, "all installed PostgreSQL FK/check constraints are validated").toBe("0");
    return;
  }
  const db = new DatabaseSync(backend.database, { readOnly: true });
  try {
    expect(db.prepare("PRAGMA integrity_check").all()).toEqual([{ integrity_check: "ok" }]);
    expect(db.prepare("PRAGMA foreign_key_check").all()).toEqual([]);
    for (const query of relationshipChecks) expect(db.prepare(query).get()?.n).toBe(0);
    backend.evidence.database_version = db
      .prepare("SELECT sqlite_version() AS version")
      .get()?.version;
  } finally {
    db.close();
  }
  if (!checkBackup) return;
  const baselineVersion = (backend.evidence.baseline_health as { version: string }).version;
  const candidateVersion = (backend.evidence.candidate_health as { version: string }).version;
  if (baselineVersion === candidateVersion) {
    backend.evidence.sqlite_backup =
      "not required: baseline and candidate have the same recorded version";
    return;
  }
  const backupDir = path.join(path.dirname(backend.database), "backups");
  const backups = fs.readdirSync(backupDir).filter((file) => file.endsWith(".db"));
  expect(backups.length, "real pre-migration SQLite backup exists").toBeGreaterThan(0);
  const snapshot = new DatabaseSync(path.join(backupDir, backups.sort().at(-1)!), {
    readOnly: true,
  });
  try {
    expect(snapshot.prepare("PRAGMA integrity_check").get()?.integrity_check).toBe("ok");
    const baseline = JSON.parse(
      fs.readFileSync(path.join(backend.root, "seed.json"), "utf8"),
    ) as UpgradeSeed;
    for (const [kind, task] of Object.entries(baseline.tasks))
      expect(snapshot.prepare("SELECT title FROM tasks WHERE id=?").get(task.id)?.title).toBe(
        `Upgrade ${kind}`,
      );
    if (seed.tasks.new)
      expect(
        snapshot.prepare("SELECT id FROM tasks WHERE id=?").get(seed.tasks.new.id),
      ).toBeUndefined();
  } finally {
    snapshot.close();
  }
}

export async function waitForUpgradeArchive(backend: UpgradeBackend, taskId: string) {
  if (!/^[a-f0-9-]{36}$/.test(taskId)) throw new Error("invalid owned task identity");
  const query = `SELECT state FROM task_resource_cleanup_jobs WHERE task_id='${taskId}'`;
  await expect
    .poll(
      () => {
        let states: string[];
        if (backend.engine === "postgres")
          states = postgresSQL(backend.postgresName, query).split("\n").filter(Boolean);
        else {
          const db = new DatabaseSync(backend.database, { readOnly: true });
          try {
            states = db
              .prepare(query)
              .all()
              .map((row: { state: unknown }) => String(row.state));
          } finally {
            db.close();
          }
        }
        if (states.includes("failed")) throw new Error(`archive cleanup failed: ${taskId}`);
        return (
          states.length > 0 && states.every((state) => ["succeeded", "cancelled"].includes(state))
        );
      },
      { timeout: 60_000, message: "archive cleanup must settle before unarchive" },
    )
    .toBe(true);
}
