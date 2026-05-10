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

function matchPnpmSetupUsesLine(line: string): RegExpMatchArray | null {
  return line.match(/^(\s*)(?:-\s*)?uses:\s*["']?pnpm\/action-setup@v\d+["']?\s*(?:#.*)?$/);
}

function findStepIndent(lines: string[], usesLineIndex: number, usesIndent: number): number {
  if (lines[usesLineIndex].trimStart().startsWith("- ")) {
    return usesIndent;
  }

  for (let index = usesLineIndex - 1; index >= 0; index -= 1) {
    const line = lines[index];
    if (isBlankOrComment(line)) {
      continue;
    }

    const lineIndent = indentation(line);
    if (lineIndent < usesIndent && line.slice(lineIndent).startsWith("- ")) {
      return lineIndent;
    }
  }

  return Math.max(0, usesIndent - 2);
}

function extractWorkflowPnpmVersions(workflow: string): Array<string | undefined> {
  const lines = workflow.split(/\r?\n/);
  const versions: Array<string | undefined> = [];

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index];
    const setupMatch = matchPnpmSetupUsesLine(line);
    if (setupMatch !== null) {
      const stepIndent = findStepIndent(lines, index, setupMatch[1].length);
      versions.push(findPnpmSetupVersion(lines, index + 1, stepIndent));
    }
  }

  return versions;
}

function assertWorkflowPnpmVersions(file: string, expectedVersion: string): number {
  const versions = extractWorkflowPnpmVersions(readRepoFile(file));
  for (const version of versions) {
    if (version === undefined) {
      expect(version, `${file}: pnpm/action-setup step is missing a version pin`).toBeDefined();
      continue;
    }

    expect(version, `${file}: pnpm/action-setup version must match Dockerfile PNPM_VERSION`).toBe(
      expectedVersion,
    );
  }

  return versions.length;
}

describe("release package manager version", () => {
  it("pins pnpm consistently for Docker and GitHub Actions", () => {
    const dockerfile = readRepoFile("Dockerfile");
    const dockerPnpmVersion = extractDockerPnpmVersion(dockerfile);

    expect(dockerfile).not.toContain("pnpm@latest");
    expect(dockerPnpmVersion, "Dockerfile: PNPM_VERSION must be pinned").toBeDefined();
    if (dockerPnpmVersion === undefined) {
      throw new Error("Dockerfile: PNPM_VERSION must be pinned");
    }

    const workflowSetupCount = workflowFiles().reduce(
      (count, file) => count + assertWorkflowPnpmVersions(file, dockerPnpmVersion),
      0,
    );
    expect(workflowSetupCount).toBeGreaterThan(0);
  });
});
