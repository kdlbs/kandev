import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import type { KubernetesCluster, KubernetesPod } from "../fixtures/kubernetes-tools";
import { waitForKubernetesPod, waitForTaskResourceCleanupAttempt } from "../helpers/kubernetes";
import { DatabaseSync } from "../helpers/node-sqlite";

const tempRoots: string[] = [];

afterEach(() => {
  for (const root of tempRoots.splice(0)) {
    fs.rmSync(root, { recursive: true, force: true });
  }
});

function podSnapshot(ready: boolean, running: boolean): KubernetesPod {
  return {
    metadata: {
      name: "kandev-session-1",
      uid: "pod-uid-1",
      labels: {
        "kandev.ai/task-id": "task-1",
        "kandev.ai/session-id": "session-1",
      },
    },
    status: {
      phase: running ? "Running" : "Pending",
      containerStatuses: [
        {
          name: "kandev-agent",
          ready,
          restartCount: 0,
          state: running
            ? { running: { startedAt: "2026-08-24T22:00:00Z" } }
            : { waiting: { reason: "ContainerCreating" } },
        },
      ],
    },
  } as KubernetesPod;
}

describe("Kubernetes E2E causal helpers", () => {
  it("waits for the exact main container to be running and ready", async () => {
    const snapshots = [
      podSnapshot(false, false),
      podSnapshot(false, true),
      podSnapshot(true, true),
    ];
    let reads = 0;
    const cluster = {
      namespace: "kandev-e2e-workloads",
      json: () => ({ items: [snapshots[Math.min(reads++, snapshots.length - 1)]] }),
    } as unknown as KubernetesCluster;

    const pod = await waitForKubernetesPod(cluster, "task-1", "session-1", 2_000);

    expect(reads).toBe(3);
    expect(
      pod.status?.containerStatuses?.find((container) => container.name === "kandev-agent")?.ready,
    ).toBe(true);
  });

  it("observes the newest archive-family cleanup retry with its durable aggregate error", async () => {
    const backendTmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-cleanup-helper-"));
    tempRoots.push(backendTmpDir);
    const database = path.join(backendTmpDir, "kandev.db");
    const sqlite = new DatabaseSync(database);
    try {
      sqlite.exec(`CREATE TABLE task_resource_cleanup_jobs (
        task_id TEXT NOT NULL,
        trigger TEXT NOT NULL,
        state TEXT NOT NULL,
        attempts INTEGER NOT NULL,
        last_error TEXT NOT NULL,
        created_at TEXT NOT NULL
      );
      INSERT INTO task_resource_cleanup_jobs VALUES
        ('task-1', 'archive', 'succeeded', 1, '', '2026-08-24T21:00:00Z'),
        ('task-1', 'cascade_archive', 'retry_wait', 1,
         '1 runtime stop operations failed', '2026-08-24T22:00:00Z');`);
    } finally {
      sqlite.close();
    }

    const originalPath = process.env.PATH;
    process.env.PATH = "";
    let cleanup;
    try {
      cleanup = await waitForTaskResourceCleanupAttempt(
        backendTmpDir,
        "task-1",
        /runtime stop operations failed/i,
        2_000,
      );
    } finally {
      process.env.PATH = originalPath;
    }

    expect(cleanup).toMatchObject({
      attempts: 1,
      last_error: "1 runtime stop operations failed",
      state: "retry_wait",
    });
  });
});
