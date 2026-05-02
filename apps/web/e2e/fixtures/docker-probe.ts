import { execFileSync, spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

/**
 * Image used by the Docker E2E project. Built once per machine and reused.
 * Tag is stable so layer caches survive across runs.
 */
export const E2E_IMAGE_TAG = "kandev-agent:e2e";

const E2E_DOCKERFILE = `FROM node:22-slim
RUN apt-get update \\
 && apt-get install -y --no-install-recommends git ca-certificates curl \\
 && rm -rf /var/lib/apt/lists/*
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
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-e2e-image-"));
  try {
    fs.writeFileSync(path.join(tmpDir, "Dockerfile"), E2E_DOCKERFILE);
    execFileSync("docker", ["build", "-t", E2E_IMAGE_TAG, tmpDir], {
      stdio: process.env.E2E_DEBUG ? "inherit" : "ignore",
    });
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

/**
 * Remove all containers labeled as managed by kandev. Called at worker
 * teardown to clean up anything the suite leaked. Safe to run on a host with
 * no matching containers.
 */
export function removeKandevContainers(): void {
  const list = spawnSync("docker", ["ps", "-aq", "--filter", "label=kandev.managed=true"], {
    encoding: "utf8",
  });
  if (list.status !== 0) return;
  const ids = list.stdout
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  if (ids.length === 0) return;
  spawnSync("docker", ["rm", "-f", ...ids], { stdio: "ignore" });
}
