import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { tmpdir } from "node:os";
import { pathToFileURL } from "node:url";
import { describe, expect, it } from "vitest";

const WEB_ROOT = path.resolve(import.meta.dirname, "..");
const CONFIG_PATH = path.join(WEB_ROOT, "vitest.config.ts");
const SELECTION_PATH = path.join(import.meta.dirname, "vitest-project-selection.ts");
const NODE_PROJECT = "node" as const;
const BROWSER_PROJECT = "browser" as const;
const BROWSER_LOCALES_PROJECT = "browser-locales" as const;
const FIXTURE_TEST_SOURCE = "export const testValue = true;\n";
const FIXTURE_PLAYWRIGHT_SOURCE = "export const shouldNotRun = true;\n";

type SelectionModule = typeof import("./vitest-project-selection");

async function loadSelection(): Promise<SelectionModule> {
  const config = readFileSync(CONFIG_PATH, "utf8");
  expect(config).toContain("projects:");
  expect(config).toContain("projectDefinitions.browser.include");
  expect(config).toContain('projectDefinitions["browser-locales"].include');
  expect(existsSync(SELECTION_PATH)).toBe(true);
  return import(/* @vite-ignore */ pathToFileURL(SELECTION_PATH).href);
}

function withFixture<T>(files: Record<string, string>, callback: (root: string) => T): T {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-vitest-selection-"));

  try {
    for (const [relativeFile, contents] of Object.entries(files)) {
      const file = path.join(root, relativeFile);
      mkdirSync(path.dirname(file), { recursive: true });
      writeFileSync(file, contents);
    }

    return callback(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

describe("Vitest project selection", () => {
  it("keeps every test file in exactly one project", async () => {
    const { discoverTestFiles, matchingProjects } = await loadSelection();
    const files = discoverTestFiles(WEB_ROOT);

    expect(files.length).toBeGreaterThan(1000);
    expect(files.some((file) => file.startsWith("e2e/") && file.endsWith(".test.ts"))).toBe(true);

    for (const file of files) {
      expect(matchingProjects(file), file).toHaveLength(1);
    }
  });

  it("keeps the projects disjoint while retaining the complete file identity set", async () => {
    const { discoverTestFiles, matchingProjects, projectFiles } = await loadSelection();
    const files = discoverTestFiles(WEB_ROOT);
    const selected = new Set(files.flatMap((file) => matchingProjects(file)));

    expect(selected).toEqual(new Set([NODE_PROJECT, BROWSER_PROJECT, BROWSER_LOCALES_PROJECT]));
    expect(new Set(projectFiles.node)).toEqual(
      new Set(files.filter((file) => matchingProjects(file)[0] === NODE_PROJECT)),
    );
    expect(new Set(projectFiles.browser)).toEqual(
      new Set(files.filter((file) => matchingProjects(file)[0] === BROWSER_PROJECT)),
    );
    expect(new Set(projectFiles["browser-locales"])).toEqual(
      new Set(files.filter((file) => matchingProjects(file)[0] === BROWSER_LOCALES_PROJECT)),
    );
  });

  it("matches configured project include and exclude rules for fixture identities", async () => {
    const {
      PROJECT_NAMES,
      buildProjectSelection,
      discoverTestFiles,
      matchingProjectsForSelection,
      matchesConfiguredProject,
      matchesTestFileGlob,
    } = await loadSelection();

    withFixture(
      {
        "scripts/unclassified.test.ts": FIXTURE_TEST_SOURCE,
        "components/indirect-locale.test.ts":
          'import "./indirect-locale-helper";\nexport const testValue = true;\n',
        "components/indirect-locale-helper.ts":
          'export function setLocale() { return changeLanguage("en"); }\n',
        "components/unknown.spec.cjsx": FIXTURE_TEST_SOURCE,
        "components/unknown.spec.ctsx": FIXTURE_TEST_SOURCE,
        "components/unknown.test.cts": FIXTURE_TEST_SOURCE,
        "components/unknown.test.mjsx": FIXTURE_TEST_SOURCE,
        "e2e/ignored.spec.ts": FIXTURE_PLAYWRIGHT_SOURCE,
        "e2e/fixtures/ignored.test.ts": FIXTURE_PLAYWRIGHT_SOURCE,
        "e2e/pages/ignored.test.ts": FIXTURE_PLAYWRIGHT_SOURCE,
      },
      (root) => {
        const selection = buildProjectSelection(root);
        const files = discoverTestFiles(root);

        expect(files).toEqual([
          "components/indirect-locale.test.ts",
          "components/unknown.spec.cjsx",
          "components/unknown.spec.ctsx",
          "components/unknown.test.cts",
          "components/unknown.test.mjsx",
          "scripts/unclassified.test.ts",
        ]);
        for (const file of [
          "components/unknown.spec.cjsx",
          "components/unknown.spec.ctsx",
          "components/unknown.test.cts",
          "components/unknown.test.mjsx",
        ]) {
          expect(matchesTestFileGlob(file)).toBe(true);
        }

        for (const file of files) {
          const configuredProjects = PROJECT_NAMES.filter((project) =>
            matchesConfiguredProject(file, project, selection),
          );

          expect(configuredProjects).toEqual(matchingProjectsForSelection(file, selection));
          expect(configuredProjects, file).toHaveLength(1);
        }

        expect(matchingProjectsForSelection("scripts/unclassified.test.ts", selection)).toEqual([
          BROWSER_LOCALES_PROJECT,
        ]);
        expect(
          matchingProjectsForSelection("components/indirect-locale.test.ts", selection),
        ).toEqual([BROWSER_LOCALES_PROJECT]);
        expect(matchesConfiguredProject("e2e/ignored.spec.ts", BROWSER_PROJECT, selection)).toBe(
          false,
        );
        expect(
          matchesConfiguredProject("e2e/ignored.spec.ts", BROWSER_LOCALES_PROJECT, selection),
        ).toBe(false);
      },
    );
  });

  it("gives node-only configuration tests enough time under project contention", () => {
    const config = readFileSync(CONFIG_PATH, "utf8");

    expect(config).toContain('name: "node"');
    expect(config).toContain("testTimeout: 15_000");
  });
});
