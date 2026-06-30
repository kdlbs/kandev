import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, it, expect } from "vitest";

const OUTPUT_FILE = join(__dirname, "..", "generated", "licenses.json");

type LicenseEntry = {
  name: string;
  version: string;
  license: string;
  repository?: string;
  license_text?: string;
  stale?: boolean;
  ecosystem: "npm" | "go";
};

const ECOSYSTEMS = new Set(["npm", "go"]);
const REQUIRED_FIELDS = ["name", "version", "license", "ecosystem"] as const;

function loadEntries(): LicenseEntry[] | null {
  if (!existsSync(OUTPUT_FILE)) return null;
  const raw = readFileSync(OUTPUT_FILE, "utf-8");
  return JSON.parse(raw) as LicenseEntry[];
}

function compareKeys(a: LicenseEntry, b: LicenseEntry): number {
  if (a.ecosystem !== b.ecosystem) return a.ecosystem < b.ecosystem ? -1 : 1;
  if (a.name !== b.name) return a.name < b.name ? -1 : 1;
  if (a.version !== b.version) return a.version < b.version ? -1 : 1;
  return 0;
}

describe("generated/licenses.json", () => {
  const entries = loadEntries();

  it("parses as a JSON array (or is absent in fresh checkouts)", () => {
    if (!entries) {
      // Generator hasn't run yet — skip the rest. CI runs `licenses:gen` first.
      expect(existsSync(OUTPUT_FILE)).toBe(false);
      return;
    }
    expect(Array.isArray(entries)).toBe(true);
  });

  if (!entries) return;

  it("every entry has the required fields and a valid ecosystem", () => {
    for (const entry of entries) {
      for (const field of REQUIRED_FIELDS) {
        expect(entry, `missing ${field} in ${JSON.stringify(entry)}`).toHaveProperty(field);
        expect(typeof entry[field]).toBe("string");
        expect(entry[field].length).toBeGreaterThan(0);
      }
      expect(ECOSYSTEMS.has(entry.ecosystem)).toBe(true);
      if (entry.repository !== undefined) {
        expect(typeof entry.repository).toBe("string");
      }
      if (entry.license_text !== undefined) {
        expect(typeof entry.license_text).toBe("string");
      }
      if (entry.stale !== undefined) {
        expect(typeof entry.stale).toBe("boolean");
      }
    }
  });

  it("is sorted by (ecosystem, name, version)", () => {
    for (let i = 1; i < entries.length; i++) {
      const prev = entries[i - 1];
      const curr = entries[i];
      expect(
        compareKeys(prev, curr),
        `out-of-order at index ${i}: ${prev.ecosystem}/${prev.name}@${prev.version} vs ${curr.ecosystem}/${curr.name}@${curr.version}`,
      ).toBeLessThan(0);
    }
  });

  it("has no duplicate (ecosystem, name, version) tuples", () => {
    const seen = new Set<string>();
    for (const entry of entries) {
      const key = `${entry.ecosystem}|${entry.name}|${entry.version}`;
      expect(seen.has(key), `duplicate entry ${key}`).toBe(false);
      seen.add(key);
    }
  });

  it("contains entries from both ecosystems (sanity check)", () => {
    const eco = new Set(entries.map((e) => e.ecosystem));
    expect(eco.has("npm")).toBe(true);
    expect(eco.has("go")).toBe(true);
  });
});
