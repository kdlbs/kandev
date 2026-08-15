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

const E2E_DOCKERFILE = `FROM node:22-slim
RUN apt-get update \\
 && apt-get install -y --no-install-recommends git ca-certificates curl \\
 && rm -rf /var/lib/apt/lists/*
COPY --chmod=0755 fake-lsp-server.mjs /usr/local/bin/kotlin-lsp
WORKDIR /workspace
`;

const E2E_ALPINE_DOCKERFILE = `FROM alpine:latest
RUN apk add --no-cache bash ca-certificates curl git nodejs npm
COPY --chmod=0755 fake-lsp-server.mjs /usr/local/bin/kotlin-lsp
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
    execFileSync("docker", ["build", "-t", tag, tmpDir], {
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
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

export function removeScopedKandevContainers(scope = E2E_DOCKER_SCOPE): void {
  const ids = scopedKandevContainerIDs(scope);
  if (ids.length === 0) return;
  spawnSync("docker", ["rm", "-f", ...ids], { stdio: "ignore" });
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
    removeScopedKandevContainers(scope);
    if (scopedKandevContainerIDs(scope).length === 0) return;
    await dwell(
      100,
      "poll-interval",
      "sampling interval for the container-teardown loop above; docker rm is asynchronous and the CLI reports completion only by the container list emptying",
    );
  }
  removeScopedKandevContainers(scope);
  const remaining = scopedKandevContainerIDs(scope);
  if (remaining.length > 0) {
    throw new Error(
      `E2E cleanup left ${remaining.length} process-owned Docker container(s): ${remaining.join(", ")}`,
    );
  }
}
