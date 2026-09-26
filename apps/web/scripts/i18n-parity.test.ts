import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

import { describe, expect, it } from "vitest";

const SCRIPT = path.resolve(import.meta.dirname, "i18n-parity.mjs");

/** `{ locale: { namespace: { key: value } } }` — materialised as a locales tree. */
type Fixture = Record<string, Record<string, Record<string, string>>>;

interface ParityResult {
  status: number | null;
  stdout: string;
  stderr: string;
  /** Header row locale names, in printed order, excluding the `en` denominator. */
  locales: string[];
  /** `namespace.json` -> `{ en: <denominator>, <locale>: <missing count> }`. */
  rows: Record<string, Record<string, number>>;
  /** Lines under "EXTRA KEYS". */
  extras: string[];
  /** Lines under "ABSENT NAMESPACES". */
  absent: string[];
}

/**
 * Parse the human table back into structure. Asserting on the rendered report
 * (rather than an internal API) is deliberate: the printed table IS the product
 * here, so a change that silently drops the denominator column should fail.
 */
function parseReport(stdout: string): Pick<ParityResult, "locales" | "rows" | "extras" | "absent"> {
  const lines = stdout.split("\n");
  const headerIndex = lines.findIndex((l) => /^namespace\s+en\b/.test(l));
  const locales = headerIndex === -1 ? [] : lines[headerIndex].trim().split(/\s+/).slice(2);

  const rows: Record<string, Record<string, number>> = {};
  for (const line of lines) {
    const match = /^(\S+\.json|TOTAL)\s+(\d+(?:\s+\d+)*)\s*$/.exec(line);
    if (!match) continue;
    const counts = match[2].trim().split(/\s+/).map(Number);
    rows[match[1]] = Object.fromEntries(["en", ...locales].map((name, i) => [name, counts[i]]));
  }

  const section = (header: string) => {
    const start = lines.findIndex((l) => l.startsWith(header));
    if (start === -1) return [];
    const out: string[] = [];
    for (const line of lines.slice(start + 1)) {
      if (/^[A-Z]{2,}/.test(line)) break;
      const isFinding =
        /^ {2}\S/.test(line) && !/entr\(ies\)\.$/.test(line) && line.trim() !== "(none)";
      if (isFinding) out.push(line.trim());
    }
    return out;
  };

  return { locales, rows, extras: section("EXTRA KEYS"), absent: section("ABSENT NAMESPACES") };
}

function runParity(fixture: Fixture): ParityResult {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-i18n-parity-"));
  try {
    for (const [locale, namespaces] of Object.entries(fixture)) {
      mkdirSync(path.join(root, locale), { recursive: true });
      for (const [namespace, messages] of Object.entries(namespaces)) {
        writeFileSync(
          path.join(root, locale, `${namespace}.json`),
          `${JSON.stringify(messages, null, 2)}\n`,
        );
      }
    }
    const result = spawnSync(process.execPath, [SCRIPT, root], { encoding: "utf8" });
    return {
      status: result.status,
      stdout: result.stdout,
      stderr: result.stderr,
      ...parseReport(result.stdout),
    };
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

const EN_COMMON = { save: "Save", cancel: "Cancel", close: "Close" };

describe("i18n-parity", () => {
  it("reports keys missing from a locale", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      "pt-pt": { common: { save: "Guardar" } },
    });

    expect(report.rows["common.json"]).toEqual({ en: 3, "pt-pt": 2 });
    expect(report.rows.TOTAL).toEqual({ en: 3, "pt-pt": 2 });
  });

  it("reports keys present in the locale but absent from en", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      "pt-pt": { common: { ...EN_COMMON, renamedAway: "Antigo" } },
    });

    expect(report.extras).toEqual(["pt-pt / common: renamedAway"]);
    // An extra key is not a missing key; it must not inflate the missing count.
    expect(report.rows["common.json"]["pt-pt"]).toBe(0);
  });

  it("reports zero for a complete locale, and still prints the denominator", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      "pt-pt": { common: { save: "Guardar", cancel: "Cancelar", close: "Fechar" } },
    });

    expect(report.rows["common.json"]).toEqual({ en: 3, "pt-pt": 0 });
    expect(report.extras).toEqual([]);
    // The denominator is the whole point: "0 missing" over 0 keys is not a pass.
    expect(report.stdout).toContain("3 en key(s)");
  });

  it("counts a wholly-absent namespace as every key it would hold", () => {
    // Deliberately unlike check-i18n-keys.mjs, which scores this as one advisory
    // issue — the shape that let an 889-key namespace stay invisible.
    const report = runParity({
      en: { common: EN_COMMON, office: { agent: "Agent", routine: "Routine" } },
      "pt-pt": { common: { save: "Guardar", cancel: "Cancelar", close: "Fechar" } },
    });

    expect(report.rows["office.json"]["pt-pt"]).toBe(2);
    expect(report.absent).toEqual(["pt-pt / office: namespace file absent"]);
  });

  it("discovers locales from disk instead of a hardcoded list", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      "pt-pt": { common: EN_COMMON },
      fr: { common: { save: "Enregistrer" } },
      "zh-cn": { common: {} },
    });

    expect(report.locales).toEqual(["fr", "pt-pt", "zh-cn"]);
    expect(report.rows["common.json"]).toEqual({ en: 3, fr: 2, "pt-pt": 0, "zh-cn": 3 });
  });

  it("excludes en and pseudo from the locale columns", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      pseudo: { common: { save: "Śàvē" } },
      "pt-pt": { common: EN_COMMON },
    });

    expect(report.locales).toEqual(["pt-pt"]);
    expect(report.stdout).not.toContain("pseudo /");
  });

  it("exits 0 even when it finds problems, because it is a report and not a gate", () => {
    const report = runParity({
      en: { common: EN_COMMON },
      "pt-pt": { common: { stale: "Antigo" } },
    });

    expect(report.rows["common.json"]["pt-pt"]).toBe(3);
    expect(report.extras).toEqual(["pt-pt / common: stale"]);
    expect(report.status).toBe(0);
  });
});
