import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = resolve(__dirname, "../../..");

function readRepoFile(path: string): string {
  return readFileSync(resolve(repoRoot, path), "utf8");
}

function workflowFiles(): string[] {
  const workflowDir = resolve(repoRoot, ".github/workflows");
  return readdirSync(workflowDir)
    .filter((name) => name.endsWith(".yml") || name.endsWith(".yaml"))
    .map((name) => `.github/workflows/${name}`);
}

function extractDockerPnpmVersion(dockerfile: string): string | undefined {
  return dockerfile.match(/^ARG PNPM_VERSION=([0-9]+\.[0-9]+\.[0-9]+)$/m)?.[1];
}

function extractWorkflowPnpmVersions(workflow: string): string[] {
  return [...workflow.matchAll(/uses:\s+pnpm\/action-setup@v4[\s\S]*?version:\s+"([^"]+)"/g)].map(
    (match) => match[1],
  );
}

describe("release package manager version", () => {
  it("pins pnpm consistently for Docker and GitHub Actions", () => {
    const dockerfile = readRepoFile("Dockerfile");
    const dockerPnpmVersion = extractDockerPnpmVersion(dockerfile);
    const workflowVersions = workflowFiles().flatMap((file) =>
      extractWorkflowPnpmVersions(readRepoFile(file)),
    );

    expect(dockerfile).not.toContain("pnpm@latest");
    expect(dockerPnpmVersion).toBeDefined();
    expect(workflowVersions.length).toBeGreaterThan(0);
    expect(new Set(workflowVersions)).toEqual(new Set([dockerPnpmVersion]));
  });
});
