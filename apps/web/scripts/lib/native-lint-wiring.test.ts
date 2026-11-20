/**
 * The native lint configuration is actually wired up.
 *
 * RuleTester and the ESLint adapter suites prove individual rules. They cannot
 * prove that `pnpm run lint` loads the Oxlint configuration, registers the JS
 * plugins, keeps every scoped guard on its original owner paths, or preserves
 * the file exemptions. These tests execute the real CLI against an isolated
 * project that uses the checked-in configuration and the original owner
 * filenames.
 *
 * The mutation tests are deliberate: each one changes a copy of the
 * configuration to show that the guard fixture would pass silently if the
 * corresponding wiring were lost.
 */
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SLEEP_EXEMPT_FILES } from "../../eslint-rules/no-unsanctioned-sleep.mjs";

const WEB_DIR = path.resolve(import.meta.dirname, "../..");
const OXLINT_SCRIPT = path.join(WEB_DIR, "node_modules", "oxlint", "bin", "oxlint");
vi.setConfig({ testTimeout: 30_000, hookTimeout: 30_000 });

const SYSTEM_OWNER_RULE = "system-query-owner(no-migrated-system-zustand-owner)";
const MAIN_CONFIG = "oxlint.config.mjs";
const MUTANT_CONFIG = "oxlint.mutant.config.mjs";
const I18N_RULE = "i18next(no-literal-string)";
const LINK_PROBE = "components/link-probe.tsx";
const I18N_PROBE = "app/probe.tsx";
const LINKS_AUTHORITY = "lib/links.ts";

let project: string;

function copyConfigTree(target: string): void {
  for (const file of [
    MAIN_CONFIG,
    "oxlint.i18n.config.mjs",
    "oxlint.e2e-sleeps.config.mjs",
    "eslint.i18n.options.mjs",
  ]) {
    fs.copyFileSync(path.join(WEB_DIR, file), path.join(target, file));
  }
  fs.cpSync(path.join(WEB_DIR, "eslint-rules"), path.join(target, "eslint-rules"), {
    recursive: true,
  });
  fs.symlinkSync(path.join(WEB_DIR, "node_modules"), path.join(target, "node_modules"), "dir");
}

function write(relative: string, source: string): void {
  const target = path.join(project, relative);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, source, "utf8");
}

function runOxlint(
  paths: string[],
  config = MAIN_CONFIG,
): { status: number | null; output: string } {
  const result = spawnSync(
    process.execPath,
    [OXLINT_SCRIPT, "-c", config, "--deny-warnings", ...paths],
    {
      cwd: project,
      encoding: "utf8",
    },
  );
  return { status: result.status, output: `${result.stdout}${result.stderr}` };
}

function expectFinding(relative: string, rule: string): void {
  const result = runOxlint([relative]);
  expect(result.status, result.output).toBe(1);
  expect(result.output).toContain(rule);
}

function expectClean(relative: string, config = MAIN_CONFIG): void {
  const result = runOxlint([relative], config);
  expect(result.status, result.output).toBe(0);
  // Oxlint prints a success summary containing "0 errors"; only diagnostic
  // lines have the `: error <rule>` shape.
  expect(result.output).not.toMatch(/:\s*(error|warning)\s/);
}

beforeEach(() => {
  project = fs.mkdtempSync(path.join(os.tmpdir(), "kandev-native-lint-"));
  copyConfigTree(project);
});

afterEach(() => {
  fs.rmSync(project, { recursive: true, force: true });
});

describe("native rule execution", () => {
  it("rejects a native rule violation through the required configuration", () => {
    write(
      "components/debug-probe.tsx",
      "export function Probe() {\n  debugger;\n  return null;\n}\n",
    );
    expectFinding("components/debug-probe.tsx", "eslint(no-debugger)");
  });

  it("honours inline suppressions in both comment dialects", () => {
    write(
      "components/suppressed-probe.tsx",
      "export function Probe() {\n  // eslint-disable-next-line no-debugger\n  debugger;\n  // oxlint-disable-next-line no-debugger\n  debugger;\n  return null;\n}\n",
    );
    expectClean("components/suppressed-probe.tsx");
  });

  it("keeps the JavaScript-only core checks from the previous recommended set", () => {
    write(
      "scripts/unsafe-negation.mjs",
      'const key = "a";\nif (!key in { a: 1 }) {\n  console.log("x");\n}\n',
    );
    expectFinding("scripts/unsafe-negation.mjs", "eslint(no-unsafe-negation)");
    write("scripts/unreachable.js", 'export function f() {\n  return 1;\n  console.log("x");\n}\n');
    expectFinding("scripts/unreachable.js", "eslint(no-unreachable)");
  });

  it("rejects legacy octal literals through the native parser", () => {
    write("scripts/legacy-octal.js", "var mode = 0755;\nexport { mode };\n");
    const result = runOxlint(["scripts/legacy-octal.js"]);
    expect(result.status).toBe(1);
    expect(result.output).toMatch(/octal literals/i);
  });

  it("reports a TypeScript parser error instead of silently skipping the file", () => {
    write(
      "components/duplicate-method.ts",
      "export class Duplicate {\n  create() {\n    return 1;\n  }\n  create() {\n    return 2;\n  }\n}\n",
    );
    const result = runOxlint(["components/duplicate-method.ts"]);
    expect(result.status).toBe(1);
    expect(result.output).toContain("has already been declared");
  });
});

describe("i18n guard", () => {
  it("rejects literal JSX text and display attributes", () => {
    write(I18N_PROBE, "export function Probe() {\n  return <span>Hardcoded copy</span>;\n}\n");
    expectFinding(I18N_PROBE, I18N_RULE);
    write(
      "app/attribute-probe.tsx",
      'export function Probe() {\n  return <Button label="Save changes" />;\n}\n',
    );
    expectFinding("app/attribute-probe.tsx", I18N_RULE);
  });

  it("still rejects HTML entities that spell copy, and accepts the same symbols as Unicode", () => {
    write(
      "app/entity-probe.tsx",
      "export function Probe() {\n  return <span>&middot;</span>;\n}\n",
    );
    expectFinding("app/entity-probe.tsx", I18N_RULE);
    write("app/unicode-probe.tsx", "export function Probe() {\n  return <span>·</span>;\n}\n");
    expectClean("app/unicode-probe.tsx");
  });

  it("keeps the test-file exemption", () => {
    write(
      "app/probe.test.tsx",
      "export function Probe() {\n  return <span>Hardcoded copy</span>;\n}\n",
    );
    expectClean("app/probe.test.tsx");
    // The focused preview keeps the same exemption.
    const result = runOxlint(["app/probe.test.tsx"], "oxlint.i18n.config.mjs");
    expect(result.status).toBe(0);
  });

  it("keeps inline i18n suppressions effective", () => {
    write(
      "app/suppressed-i18n.tsx",
      "export function Probe() {\n  // eslint-disable-next-line i18next/no-literal-string\n  return <span>Hardcoded copy</span>;\n}\n",
    );
    expectClean("app/suppressed-i18n.tsx");
  });

  it("runs the focused preview on selected paths", () => {
    write(I18N_PROBE, "export function Probe() {\n  return <span>Hardcoded copy</span>;\n}\n");
    const result = runOxlint(["app/probe.tsx"], "oxlint.i18n.config.mjs");
    expect(result.status).toBe(1);
    expect(result.output).toContain(I18N_RULE);
  });
});

describe("task-link guard", () => {
  it("rejects a hand-built task link and keeps the authority exempt", () => {
    const probe = "export function go(id: string) {\n  window.location.href = `/t/${id}`;\n}\n";
    write(LINK_PROBE, probe);
    expectFinding(LINK_PROBE, "task-links(no-task-link-bypass)");
    write(LINKS_AUTHORITY, probe);
    expectClean(LINKS_AUTHORITY);
  });

  it("would miss the violation if the guard were disabled", () => {
    write(LINK_PROBE, 'export function go() {\n  window.location.href = "/t/1";\n}\n');
    const source = fs.readFileSync(path.join(project, MAIN_CONFIG), "utf8");
    fs.writeFileSync(
      path.join(project, MUTANT_CONFIG),
      source.replace(
        '"task-links/no-task-link-bypass": "error"',
        '"task-links/no-task-link-bypass": "off"',
      ),
      "utf8",
    );
    const result = runOxlint([LINK_PROBE], MUTANT_CONFIG);
    expect(result.status).toBe(0);
  });

  it("would report the authority if its exemption were lost", () => {
    write(LINKS_AUTHORITY, 'export const href = "/t/1";\n');
    const source = fs.readFileSync(path.join(project, MAIN_CONFIG), "utf8");
    fs.writeFileSync(
      path.join(project, MUTANT_CONFIG),
      source.replace(
        '...testFileExclusions, "e2e/**", "lib/links.ts"',
        '...testFileExclusions, "e2e/**"',
      ),
      "utf8",
    );
    const result = runOxlint([LINKS_AUTHORITY], MUTANT_CONFIG);
    expect(result.status).toBe(1);
    expect(result.output).toContain("task-links(no-task-link-bypass)");
  });
});

describe("System Query owner guard", () => {
  const fixtures: Record<string, string> = {
    "lib/state/slices/system/types.ts": "export type SystemBackupsState = { items: string[] };\n",
    "lib/state/slices/system/system-slice.ts":
      "const defaultSystemState = { system: { info: null, database: null } };\nexport { defaultSystemState };\n",
    "lib/state/slices/system/index.ts": 'export type { SystemBackupsState } from "./types";\n',
    "lib/state/hydration/hydrator.ts":
      "export function hydrateState(draft: { system: Record<string, unknown> }) {\n  draft.system.backups = null;\n}\n",
  };

  it("reports every original owner path", () => {
    for (const [relative, source] of Object.entries(fixtures)) {
      write(relative, source);
      expectFinding(relative, SYSTEM_OWNER_RULE);
    }
  });

  it("does not apply the guard to the same source at a renamed path", () => {
    write("lib/state/slices/system/types-copy.ts", fixtures["lib/state/slices/system/types.ts"]);
    expectClean("lib/state/slices/system/types-copy.ts");
  });
});

describe("TanStack Query rules", () => {
  it("rejects an incomplete query key", () => {
    write(
      "hooks/domains/query-key-probe.ts",
      'import { useQuery } from "@tanstack/react-query";\nexport function useTodo(todoId: string) {\n  return useQuery({ queryKey: ["todo"], queryFn: () => todoId });\n}\n',
    );
    expectFinding("hooks/domains/query-key-probe.ts", "@tanstack/query(exhaustive-deps)");
  });

  it("rejects rest destructuring of a query result", () => {
    write(
      "hooks/domains/query-rest-probe.ts",
      'import { useQuery } from "@tanstack/react-query";\nexport function useTodo() {\n  const { data, ...rest } = useQuery({ queryKey: ["todo"], queryFn: async () => 1 });\n  return { data, ...rest };\n}\n',
    );
    expectFinding("hooks/domains/query-rest-probe.ts", "@tanstack/query(no-rest-destructuring)");
  });

  it("rejects an unstable Query client", () => {
    write(
      "components/query-client-probe.tsx",
      'import { QueryClient, QueryClientProvider } from "@tanstack/react-query";\nexport function App() {\n  const client = new QueryClient();\n  return <QueryClientProvider client={client}><span /></QueryClientProvider>;\n}\n',
    );
    expectFinding("components/query-client-probe.tsx", "@tanstack/query(stable-query-client)");
  });

  it("rejects an unstable dependency derived from a query result", () => {
    write(
      "components/query-deps-probe.tsx",
      'import { useCallback } from "react";\nimport { useQuery } from "@tanstack/react-query";\nexport function App() {\n  const query = useQuery({ queryKey: ["todo"], queryFn: async () => 1 });\n  const refresh = useCallback(() => query.refetch(), [query]);\n  return <button onClick={refresh} />;\n}\n',
    );
    expectFinding("components/query-deps-probe.tsx", "@tanstack/query(no-unstable-deps)");
  });
});

describe("E2E wait guard and exemptions", () => {
  const exempt = SLEEP_EXEMPT_FILES.find((file) => file.endsWith(".ts"));
  if (exempt === undefined) throw new Error("expected a TypeScript sleep exemption");

  it("rejects an unconditional sleep in a spec", () => {
    write(
      "e2e/probe.spec.ts",
      "export async function go(page: { waitForTimeout: (ms: number) => Promise<void> }) {\n  await page.waitForTimeout(500);\n}\n",
    );
    expectFinding("e2e/probe.spec.ts", "e2e-sleeps(no-unsanctioned-sleep)");
  });

  it("keeps the exemption list effective in the focused config", () => {
    write(
      exempt,
      "export async function go(page: { waitForTimeout: (ms: number) => Promise<void> }) {\n  await page.waitForTimeout(500);\n}\n",
    );
    expectClean(exempt);
    const focused = runOxlint([exempt], "oxlint.e2e-sleeps.config.mjs");
    expect(focused.status).toBe(0);
  });
});

describe("configuration wiring failures cannot be swallowed", () => {
  it("fails when a registered JS plugin cannot be loaded", () => {
    const source = fs.readFileSync(path.join(project, MAIN_CONFIG), "utf8");
    fs.writeFileSync(
      path.join(project, "oxlint.missing-plugin.config.mjs"),
      source.replace("./eslint-rules/no-task-link-bypass.mjs", "./eslint-rules/not-installed.mjs"),
      "utf8",
    );
    write("components/probe.tsx", "export const value = 1;\n");
    const result = runOxlint(["components/probe.tsx"], "oxlint.missing-plugin.config.mjs");
    expect(result.status).not.toBe(0);
    expect(result.output).toContain("not-installed");
  });

  it("keeps selected paths containing spaces in scope", () => {
    write(
      "components/space probe.tsx",
      "export function Probe() {\n  debugger;\n  return null;\n}\n",
    );
    const result = runOxlint(["components/space probe.tsx"]);
    expect(result.status).toBe(1);
    expect(result.output).toContain("eslint(no-debugger)");
  });
});
