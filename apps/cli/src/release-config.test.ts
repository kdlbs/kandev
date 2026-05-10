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

function indentation(line: string): number {
  return line.search(/\S/);
}

function isBlankOrComment(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.length === 0 || trimmed.startsWith("#");
}

function parseVersionLine(line: string): string | undefined {
  const match = line.match(/^\s*version:\s*(?:"([^"]+)"|'([^']+)'|([^"'\s#]+))\s*(?:#.*)?$/);
  return match?.[1] ?? match?.[2] ?? match?.[3];
}

function findVersionInWithBlock(
  lines: string[],
  start: number,
  withIndent: number,
): string | undefined {
  for (let index = start; index < lines.length; index += 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }
    if (indentation(line) <= withIndent) {
      return undefined;
    }

    const version = parseVersionLine(line);
    if (version !== undefined) {
      return version;
    }
  }

  return undefined;
}

function findPnpmSetupVersion(
  lines: string[],
  start: number,
  stepIndent: number,
): string | undefined {
  for (let index = start; index < lines.length; index += 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }
    if (indentation(line) <= stepIndent) {
      return undefined;
    }

    const withMatch = line.match(/^(\s*)with:\s*(?:#.*)?$/);
    if (withMatch !== null) {
      return findVersionInWithBlock(lines, index + 1, withMatch[1].length);
    }
  }

  return undefined;
}

function extractWorkflowPnpmVersions(workflow: string): Array<string | undefined> {
  const lines = workflow.split(/\r?\n/);
  const versions: Array<string | undefined> = [];

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const setupMatch = line.match(/^(\s*)-\s+uses:\s*["']?pnpm\/action-setup@v4["']?\s*(?:#.*)?$/);
    if (setupMatch !== null) {
      versions.push(findPnpmSetupVersion(lines, index + 1, setupMatch[1].length));
    }
  }

  return versions;
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
    expect(workflowVersions).not.toContain(undefined);
    expect(new Set(workflowVersions)).toEqual(new Set([dockerPnpmVersion]));
  });
});
