import { expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import { createRequire } from "node:module";
import fs from "node:fs";
import path from "node:path";
import { makeGitEnv } from "./git-helper";

export type AdoptionIdentity = {
  task_id: string;
  session_id: string;
  queue_incarnation_id: string;
  task_environment_id: string;
  ownership_generation: number;
  workspace_path: string;
  slot_id: string;
  attachment_id: string;
  checkout_branch: string;
  worktree_id: string;
  worktree_branch: string;
  position: number;
  worktree_path: string;
  worktree_source_clone_path: string;
  worktree_source_common_dir: string;
};

type Database = {
  prepare(sql: string): { get(...args: unknown[]): unknown };
  close(): void;
};

const nodeRequire = createRequire(path.join(process.cwd(), "package.json"));

export function readAdoptionIdentity(tmpDir: string, sessionId: string): AdoptionIdentity {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (filename: string, options: { readOnly: boolean }) => Database;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"), { readOnly: true });
  try {
    const row = db
      .prepare(
        `SELECT ts.task_id, ts.id AS session_id, ts.queue_incarnation_id,
      te.id AS task_environment_id, te.ownership_generation, ts.workspace_path,
      er.id AS slot_id, tr.id AS attachment_id, COALESCE(tr.checkout_branch,'') AS checkout_branch, er.worktree_id, er.worktree_branch,
      er.position, er.worktree_path,
      COALESCE(er.worktree_source_clone_path,'') AS worktree_source_clone_path,
      COALESCE(er.worktree_source_common_dir,'') AS worktree_source_common_dir
      FROM task_sessions ts JOIN task_environments te ON te.id=ts.task_environment_id
      JOIN task_environment_repos er ON er.task_environment_id=te.id
      JOIN task_repositories tr ON tr.task_id=ts.task_id AND tr.repository_id=er.repository_id
      WHERE ts.id=? AND er.status='active' AND er.deleted_at IS NULL`,
      )
      .get(sessionId);
    expect(row, "real launch must persist its complete selected identity").toBeTruthy();
    return row as AdoptionIdentity;
  } finally {
    db.close();
  }
}

export function liveAdoptionConsumers(tmpDir: string, sessionId: string): number {
  const { DatabaseSync } = nodeRequire("node:sqlite") as {
    DatabaseSync: new (filename: string, options: { readOnly: boolean }) => Database;
  };
  const db = new DatabaseSync(path.join(tmpDir, "kandev.db"), { readOnly: true });
  try {
    const row = db
      .prepare(
        `SELECT COUNT(*) AS count FROM executors_running
      WHERE session_id=? AND (status IS NULL OR status NOT IN ('failed','stopped','complete'))`,
      )
      .get(sessionId) as { count: number };
    return row.count;
  } finally {
    db.close();
  }
}

export function prepareIndependentAdoptionClone(
  tmpDir: string,
  source: string,
  identity: AdoptionIdentity,
): {
  candidate: string;
  marker: Buffer;
  index: Buffer;
  sentinel: string;
  restoreAdministration: (adopted: boolean) => void;
} {
  const candidate = path.join(path.dirname(identity.worktree_path), `adopted-${randomUUID()}`);
  const env = makeGitEnv(tmpDir);
  const git = (...args: string[]) =>
    execFileSync(
      "git",
      [
        "-c",
        "core.hooksPath=/dev/null",
        "-c",
        "core.fsmonitor=false",
        "-c",
        "submodule.recurse=false",
        ...args,
      ],
      { env, encoding: "utf8" },
    );
  git("clone", "--no-local", "--branch", identity.worktree_branch, source, candidate);
  const origin = git("-C", source, "config", "--get", "remote.origin.url").trim();
  git("-C", candidate, "config", "remote.origin.url", origin);
  const sentinel = `independent checkout ${randomUUID()}\n`;
  fs.writeFileSync(path.join(candidate, "adoption-sentinel.txt"), sentinel);
  const marker = fs.readFileSync(path.join(identity.worktree_path, ".git"));
  const admin = marker
    .toString("utf8")
    .trim()
    .replace(/^gitdir:\s*/, "");
  const adminPath = path.resolve(identity.worktree_path, admin);
  expect(path.dirname(adminPath)).toBe(path.join(source, ".git", "worktrees"));
  const index = fs.readFileSync(path.join(candidate, ".git", "index"));
  const retired = fs.mkdtempSync(path.join(tmpDir, "adoption-retired-admin-"));
  const savedAdmin = path.join(retired, "administration");
  fs.renameSync(adminPath, savedAdmin);
  return {
    candidate,
    marker,
    index,
    sentinel,
    restoreAdministration: (adopted) => {
      if (!adopted) fs.renameSync(savedAdmin, adminPath);
      fs.rmSync(retired, { recursive: true });
    },
  };
}

export async function adoptionWS<T>(
  page: Page,
  baseUrl: string,
  action: string,
  payload: unknown,
): Promise<T> {
  return page.evaluate(
    ({ baseUrl, action, payload }) =>
      new Promise<T>((resolve, reject) => {
        const ws = new WebSocket(baseUrl.replace(/^http/, "ws") + "/ws");
        const id = crypto.randomUUID();
        const fail = (error: Error) => {
          clearTimeout(timer);
          ws.close();
          reject(error);
        };
        const timer = setTimeout(() => fail(new Error(`Timed out waiting for ${action}`)), 30_000);
        ws.onopen = () =>
          ws.send(
            JSON.stringify({
              id,
              type: "request",
              action,
              payload,
              timestamp: new Date().toISOString(),
            }),
          );
        ws.onerror = () => fail(new Error(`WebSocket failed for ${action}`));
        ws.onmessage = (event) => {
          const frame = JSON.parse(String(event.data));
          if (frame.id !== id) return;
          clearTimeout(timer);
          ws.close();
          if (frame.type === "error") reject(new Error(JSON.stringify(frame.payload)));
          else resolve(frame.payload as T);
        };
      }),
    { baseUrl, action, payload },
  );
}
