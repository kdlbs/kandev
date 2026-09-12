import { readdirSync } from "node:fs";
import path from "node:path";

export const PROJECT_NAMES = ["node", "browser", "browser-locales"] as const;
export type ProjectName = (typeof PROJECT_NAMES)[number];

// Keep this aligned with Vitest's default test include. A single canonical
// pattern prevents the project selector from silently dropping cjs, cts, mjs,
// mts, and their JSX variants.
export const TEST_FILE_GLOBS = ["**/*.{test,spec}.?(c|m)[jt]s?(x)"] as const;

export const BASE_TEST_EXCLUDES = [
  "e2e/**/*.spec.ts",
  "e2e/fixtures/**",
  "e2e/pages/**",
  "node_modules/**",
] as const;

const TEST_FILE_NAME = /\.(?:test|spec)\.(?:[cm]?[jt]sx?)$/;
const SKIPPED_DIRECTORIES = new Set([".git", "dist", "node_modules"]);

/**
 * These test files were reviewed as safe for the Node setup. New tests stay
 * on the full browser and locale setup until their imports are reviewed and
 * they are added here explicitly.
 */
export const REVIEWED_NODE_TEST_FILES: readonly string[] = [
  "scripts/check-i18n-keys.test.ts",
  "scripts/check-no-em-dash-ui.test.ts",
  "scripts/convert-zh-cn-to-zh-hant.test.mjs",
  "scripts/fix-trans-indices.test.ts",
  "scripts/generate-licenses-fallback.test.ts",
  "scripts/generate-licenses.test.ts",
  "scripts/i18n-parity.test.ts",
  "scripts/i18n-sweep.test.ts",
  "scripts/lib/changed-files.test.ts",
  "scripts/lib/diff-ranges.test.ts",
  "scripts/lib/e2e-sleep-ratchet.test.ts",
  "scripts/lib/e2e-sleep-wiring.test.ts",
  "scripts/lib/e2e-wait-inventory.test.ts",
  "scripts/lib/eslint-i18n-config.test.ts",
  "scripts/lib/git-base.test.ts",
  "scripts/lib/guard-allowlist.test.ts",
  "scripts/lib/nonjsx-copy.test.ts",
  "scripts/lib/nonjsx-scope.test.ts",
  "scripts/lib/removed-literals.test.ts",
  "scripts/vitest-project-selection.test.ts",
  "scripts/vitest-worker-budget.test.ts",
];

/** These browser tests were reviewed as not requiring the locale catalogs. */
export const REVIEWED_BROWSER_TEST_FILES: readonly string[] = [
  "lib/test-support/happy-dom-network.test.ts",
  "vitest-environment.test.tsx",
];

export interface ProjectFiles {
  node: string[];
  browser: string[];
  "browser-locales": string[];
}

export interface VitestProjectDefinition {
  include: readonly string[];
  exclude: readonly string[];
}

export interface ProjectSelection {
  projectFiles: ProjectFiles;
  projects: Record<ProjectName, VitestProjectDefinition>;
}

function toRelativeFile(root: string, file: string): string {
  return path.relative(root, file).split(path.sep).join("/");
}

function matchesExcludePattern(relativeFile: string, pattern: string): boolean {
  if (pattern === "e2e/**/*.spec.ts") {
    return relativeFile.startsWith("e2e/") && relativeFile.endsWith(".spec.ts");
  }
  if (pattern === "e2e/fixtures/**") return relativeFile.startsWith("e2e/fixtures/");
  if (pattern === "e2e/pages/**") return relativeFile.startsWith("e2e/pages/");
  if (pattern === "node_modules/**") return relativeFile.startsWith("node_modules/");
  return relativeFile === pattern;
}

function isExcluded(relativeFile: string): boolean {
  return BASE_TEST_EXCLUDES.some((pattern) => matchesExcludePattern(relativeFile, pattern));
}

function walkTestFiles(root: string, directory: string, files: string[]): void {
  for (const entry of readdirSync(directory, { withFileTypes: true })) {
    if (entry.isDirectory() && !SKIPPED_DIRECTORIES.has(entry.name)) {
      walkTestFiles(root, path.join(directory, entry.name), files);
      continue;
    }
    if (!entry.isFile() || !TEST_FILE_NAME.test(entry.name)) continue;
    const relativeFile = toRelativeFile(root, path.join(directory, entry.name));
    if (!isExcluded(relativeFile)) files.push(relativeFile);
  }
}

export function discoverTestFiles(root: string): string[] {
  const files: string[] = [];
  walkTestFiles(root, root, files);
  return files.sort();
}

export function matchesTestFileGlob(relativeFile: string): boolean {
  return TEST_FILE_NAME.test(relativeFile);
}

export function buildProjectFiles(root: string): ProjectFiles {
  const files = discoverTestFiles(root);
  const nodeSet = new Set(files.filter((file) => REVIEWED_NODE_TEST_FILES.includes(file)));
  const browserSet = new Set(files.filter((file) => REVIEWED_BROWSER_TEST_FILES.includes(file)));

  return {
    node: [...nodeSet].sort(),
    browser: [...browserSet].sort(),
    "browser-locales": files.filter((file) => !nodeSet.has(file) && !browserSet.has(file)),
  };
}

export function buildProjectSelection(root: string): ProjectSelection {
  const projectFiles = buildProjectFiles(root);

  return {
    projectFiles,
    projects: {
      node: {
        include: projectFiles.node,
        exclude: [...BASE_TEST_EXCLUDES],
      },
      browser: {
        include: [...TEST_FILE_GLOBS],
        exclude: [...BASE_TEST_EXCLUDES, ...projectFiles.node, ...projectFiles["browser-locales"]],
      },
      "browser-locales": {
        include: projectFiles["browser-locales"],
        exclude: [...BASE_TEST_EXCLUDES],
      },
    },
  };
}

function matchesIncludePattern(relativeFile: string, pattern: string): boolean {
  if (pattern === TEST_FILE_GLOBS[0]) return matchesTestFileGlob(relativeFile);
  return relativeFile === pattern;
}

export function matchesConfiguredProject(
  relativeFile: string,
  project: ProjectName,
  selection: ProjectSelection,
): boolean {
  const definition = selection.projects[project];
  return (
    definition.include.some((pattern) => matchesIncludePattern(relativeFile, pattern)) &&
    !definition.exclude.some((pattern) => matchesExcludePattern(relativeFile, pattern))
  );
}

const DEFAULT_ROOT = path.resolve(import.meta.dirname, "..");
export const defaultProjectSelection = buildProjectSelection(DEFAULT_ROOT);
export const projectFiles = defaultProjectSelection.projectFiles;
export const projectDefinitions = defaultProjectSelection.projects;

export function matchingProjectsForSelection(
  relativeFile: string,
  selection: ProjectSelection,
): ProjectName[] {
  return PROJECT_NAMES.filter((project) =>
    matchesConfiguredProject(relativeFile, project, selection),
  );
}

export function matchingProjects(relativeFile: string): ProjectName[] {
  return matchingProjectsForSelection(relativeFile, defaultProjectSelection);
}
