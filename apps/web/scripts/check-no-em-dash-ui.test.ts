import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import {
  EM_DASH,
  containsEmDash,
  findSourceViolations,
  scanUiEmDashViolations,
  stripCommentsPreservingStrings,
} from "./check-no-em-dash-ui.mjs";

const temporaryDirectories: string[] = [];

afterEach(() => {
  for (const directory of temporaryDirectories.splice(0)) {
    rmSync(directory, { recursive: true, force: true });
  }
});

describe("check-no-em-dash-ui", () => {
  it("ignores em dashes in comments but preserves string content", () => {
    const source = [
      `// ignored ${EM_DASH}`,
      `const copy = "visible ${EM_DASH}"; /* ignored ${EM_DASH} */`,
    ].join("\n");

    expect(stripCommentsPreservingStrings(source)).not.toContain(`ignored ${EM_DASH}`);
    expect(findSourceViolations(source, "/repo/apps/web/components/example.tsx", "/repo")).toEqual([
      { kind: "source", file: "apps/web/components/example.tsx", line: 2 },
    ]);
  });

  it("recognizes escaped em dashes that render from source literals", () => {
    expect(containsEmDash(String.raw`visible \u2014 copy`)).toBe(true);
  });

  it("ignores regex literals while stripping comments", () => {
    const source = [
      "const pattern = /``` " + EM_DASH + "/;",
      `// ignored ${EM_DASH}`,
      `const copy = "visible ${EM_DASH}";`,
    ].join("\n");

    expect(findSourceViolations(source, "/repo/apps/web/components/example.tsx", "/repo")).toEqual([
      { kind: "source", file: "apps/web/components/example.tsx", line: 3 },
    ]);
  });

  it("scans locale values, rendered source, and changelog copy", () => {
    const repoRoot = mkdtempSync(path.join(os.tmpdir(), "kandev-em-dash-"));
    temporaryDirectories.push(repoRoot);
    const webRoot = path.join(repoRoot, "apps", "web");
    mkdirSync(path.join(webRoot, "src", "locales", "en"), { recursive: true });
    mkdirSync(path.join(webRoot, "components"), { recursive: true });
    writeFileSync(
      path.join(webRoot, "src", "locales", "en", "settings.json"),
      JSON.stringify({ label: `Bad ${EM_DASH} copy` }),
    );
    writeFileSync(
      path.join(webRoot, "components", "settings.tsx"),
      `export const label = "Bad ${EM_DASH} copy";\n`,
    );
    writeFileSync(path.join(repoRoot, "CHANGELOG.md"), `- Bad ${EM_DASH} release note\n`);

    expect(scanUiEmDashViolations({ repoRoot, webRoot })).toEqual([
      { kind: "catalog", file: "apps/web/src/locales/en/settings.json", key: "label" },
      { kind: "source", file: "apps/web/components/settings.tsx", line: 1 },
      { kind: "changelog", file: "CHANGELOG.md", line: 1 },
    ]);
  });
});
