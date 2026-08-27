import { execFileSync, spawnSync } from "node:child_process";
import { randomUUID } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { dwell } from "../helpers/causal-waits";

/**
 * Image used by the Docker E2E project. Built once per machine and reused.
 * Tag is stable so layer caches survive across runs.
 */
export const E2E_IMAGE_TAG = "kandev-agent:e2e";
/** Image that deliberately keeps Alpine's BusyBox utilities for runtime compatibility tests. */
export const E2E_ALPINE_IMAGE_TAG = "kandev-agent:e2e-alpine";
/** Label value shared by resources created in this Playwright process. */
export const E2E_DOCKER_SCOPE = `e2e-${process.pid}-${randomUUID().slice(0, 8)}`;
const FAKE_LSP_SERVER = path.resolve(__dirname, "fake-lsp-server.mjs");
const MANAGED_RUNTIME_NPX_WRAPPER = path.resolve(__dirname, "managed-runtime-npx.sh");
const SCOPED_CONTAINER_CLEANUP_TIMEOUT_MS = 30_000;
const SCOPED_CONTAINER_CLEANUP_POLL_MS = 250;
const SCOPED_CONTAINER_CLEANUP_EMPTY_POLLS = 8;

const E2E_DOCKERFILE = `FROM node:22-slim
RUN apt-get update \\
 && apt-get install -y --no-install-recommends git ca-certificates curl \\
 && rm -rf /var/lib/apt/lists/*
COPY --chmod=0755 fake-lsp-server.mjs /usr/local/bin/kotlin-lsp
RUN rm -f /usr/local/bin/npx
COPY --chmod=0755 managed-runtime-npx.sh /usr/local/bin/npx
WORKDIR /workspace
`;

const E2E_ALPINE_DOCKERFILE = `FROM alpine:latest
RUN apk add --no-cache bash ca-certificates curl git nodejs npm
COPY --chmod=0755 fake-lsp-server.mjs /usr/local/bin/kotlin-lsp
RUN rm -f /usr/local/bin/npx
COPY --chmod=0755 managed-runtime-npx.sh /usr/local/bin/npx
WORKDIR /workspace
`;

/**
 * Returns true when a Docker daemon is reachable. Used by the docker fixture
 * to skip the worker when the host has no Docker (CI without docker service,
 * local dev without daemon running).
 */
export function hasDocker(): boolean {
  const result = spawnSync("docker", ["info"], { stdio: "ignore" });
  return result.status === 0;
}

/**
 * Build the kandev-agent:e2e image. Idempotent: Docker layer caching makes
 * repeated builds near-instant when the Dockerfile hasn't changed.
 */
export function buildE2EImage(): void {
  buildImage(E2E_IMAGE_TAG, E2E_DOCKERFILE);
}

/**
 * Build the Alpine image used by the Docker bootstrap compatibility test.
 * Do not install coreutils here: the test must execute BusyBox timeout.
 */
export function buildAlpineE2EImage(): void {
  buildImage(E2E_ALPINE_IMAGE_TAG, E2E_ALPINE_DOCKERFILE);
}

function buildImage(tag: string, dockerfile: string): void {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-image-"));
  try {
    fs.writeFileSync(path.join(tmpDir, "Dockerfile"), dockerfile);
    fs.copyFileSync(FAKE_LSP_SERVER, path.join(tmpDir, "fake-lsp-server.mjs"));
    fs.copyFileSync(MANAGED_RUNTIME_NPX_WRAPPER, path.join(tmpDir, "managed-runtime-npx.sh"));
    execFileSync("docker", ["build", "-t", tag, tmpDir], {
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function listScopedKandevContainers(scope: string, scanAll = false): string[] | null {
  const list = spawnSync(
    "docker",
    ["ps", "-aq", "--filter", `label=kandev.e2e.run=${scope}`, "--no-trunc"],
    { encoding: "utf8", timeout: 5_000 },
  );
  if (list.status !== 0) return null;
  const ids = list.stdout
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  if (ids.length > 0 || !scanAll) return ids;

  // Docker can briefly return an empty label-filtered list while a just
  // stopped container is becoming visible to the daemon's index. On the
  // second and later empty polls, inspect the daemon's IDs and filter labels ourselves.
  // This remains safe for concurrent shards because the fallback below only
  // returns exact managed/run label matches to the scoped remover.
  const all = spawnSync("docker", ["ps", "-aq", "--no-trunc"], {
    encoding: "utf8",
    timeout: 5_000,
  });
  if (all.status !== 0) return null;
  const allIDs = all.stdout
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  if (allIDs.length === 0) return [];

  const matchingIDs: string[] = [];
  for (const id of allIDs) {
    const inspected = spawnSync(
      "docker",
      [
        "inspect",
        "--format",
        '{{.Id}}\t{{index .Config.Labels "kandev.managed"}}\t{{index .Config.Labels "kandev.e2e.run"}}',
        id,
      ],
      { encoding: "utf8", timeout: 1_000 },
    );
    if (inspected.status !== 0) continue;
    const [fullID, managed, run] = inspected.stdout.trim().split("\t");
    if (managed === "true" && run === scope) matchingIDs.push(fullID || id);
  }
  return matchingIDs;
}

function removeContainerIDs(ids: string[]): void {
  for (const id of ids) {
    spawnSync("docker", ["rm", "-f", id], { stdio: "ignore", timeout: 5_000 });
  }
}

function discardMissingContainerIDs(pendingIDs: Set<string>): void {
  for (const id of pendingIDs) {
    const inspected = spawnSync("docker", ["inspect", id], {
      stdio: "ignore",
      timeout: 5_000,
    });
    if (inspected.status !== 0) pendingIDs.delete(id);
  }
}

/**
 * Remove only containers owned by this E2E process. Never use a daemon-wide
 * kandev.managed=true sweep: another shard can share the Docker daemon.
 */
function scopedKandevContainerIDs(scope: string): string[] {
  const list = spawnSync(
    "docker",
    [
      "ps",
      "-aq",
      "--filter",
      "label=kandev.managed=true",
      "--filter",
      `label=kandev.e2e.run=${scope}`,
    ],
    { encoding: "utf8" },
  );
  if (list.status !== 0) return [];
  return list.stdout
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
}

export async function removeScopedKandevContainers(scope = E2E_DOCKER_SCOPE): Promise<void> {
  const deadline = Date.now() + SCOPED_CONTAINER_CLEANUP_TIMEOUT_MS;
  let emptyPolls = 0;
  let lastIDs: string[] = [];
  const pendingIDs = new Set<string>();

  while (Date.now() < deadline) {
    let currentIDs = listScopedKandevContainers(scope);
    if (currentIDs?.length === 0 && emptyPolls >= 1) {
      currentIDs = listScopedKandevContainers(scope, true);
    }
    if (currentIDs) {
      lastIDs = currentIDs;
      for (const id of lastIDs) pendingIDs.add(id);
      removeContainerIDs(lastIDs);

      // `docker ps` can stop reporting a container while the daemon is still
      // completing `rm -f`. Verify every ID we observed with inspect before
      // declaring the scope empty, otherwise the next test can see the old
      // container during that removal window.
      discardMissingContainerIDs(pendingIDs);

      if (lastIDs.length === 0 && pendingIDs.size === 0) {
        emptyPolls += 1;
        // Docker's label index can lag behind a stopped container by more than
        // one poll under CI load. Require a stable empty window before the
        // next test starts so a newly visible scoped container is not leaked.
        if (emptyPolls >= SCOPED_CONTAINER_CLEANUP_EMPTY_POLLS) return;
      } else {
        emptyPolls = 0;
      }
    }

    await dwell(
      SCOPED_CONTAINER_CLEANUP_POLL_MS,
      "poll-interval",
      "Docker label visibility has no event notification",
    );
  }

  throw new Error(
    `timed out removing process-scoped Kandev containers: ${[...pendingIDs, ...lastIDs].join(", ") || "docker unavailable"}`,
  );
}

/**
 * Re-drive scoped cleanup until Docker no longer reports an owned container.
 * Docker cleanup can overlap the backend's task teardown under CI load, so a
 * single best-effort rm is not a sufficient fixture boundary.
 */
export async function waitForScopedKandevContainersRemoved(
  scope = E2E_DOCKER_SCOPE,
  timeoutMs = 30_000,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    await removeScopedKandevContainers(scope);
    if (scopedKandevContainerIDs(scope).length === 0) return;
    await dwell(
      100,
      "poll-interval",
      "sampling interval for the container-teardown loop above; docker rm is asynchronous and the CLI reports completion only by the container list emptying",
    );
  }
  await removeScopedKandevContainers(scope);
  const remaining = scopedKandevContainerIDs(scope);
  if (remaining.length > 0) {
    throw new Error(
      `E2E cleanup left ${remaining.length} process-owned Docker container(s): ${remaining.join(", ")}`,
    );
  }
}
