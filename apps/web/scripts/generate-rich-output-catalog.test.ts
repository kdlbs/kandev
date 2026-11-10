import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, it } from "vitest";
import { generateCatalog } from "./generate-rich-output-catalog";
// @covers AC-AGENTS-AGENT-RICH-OUTPUT-002.2, AC-AGENTS-AGENT-RICH-OUTPUT-002.10
it("generates deterministic schemas and detects drift without rewriting files", () => {
  const directory = mkdtempSync(join(tmpdir(), "rich-output-catalog-"));
  try {
    const target = join(directory, "schema.json");
    generateCatalog(target, false);
    const initial = readFileSync(target, "utf8");
    generateCatalog(target, false);
    expect(readFileSync(target, "utf8")).toBe(initial);
    generateCatalog(target, true);
    writeFileSync(target, "stale");
    expect(() => generateCatalog(target, true)).toThrow("drifted");
    expect(readFileSync(target, "utf8")).toBe("stale");
  } finally {
    rmSync(directory, { recursive: true });
  }
});

// @covers AC-AGENTS-AGENT-RICH-OUTPUT-003.11
it("generates checked guidance beside the schema, including column and source shapes", () => {
  const directory = mkdtempSync(join(tmpdir(), "rich-output-guidance-"));
  try {
    const target = join(directory, "schema.json");
    generateCatalog(target, false);
    const guidanceTarget = join(directory, "rich_output_catalog_v1.guidance.txt");
    expect(() => readFileSync(guidanceTarget, "utf8")).not.toThrow();
    const guidance = readFileSync(guidanceTarget, "utf8");
    expect(guidance).toContain("columns:{key,label}[]");
    expect(guidance).toContain("rows:string[][] or csv:{path,repo?}");
    expect(guidance).toContain("leaves children:[]");
    expect(guidance).toContain(
      "items:{label,status:passed|failed|running|pending|skipped,detail?}[]",
    );
    expect(Array.from(guidance.trim()).length).toBeLessThanOrEqual(650);
    writeFileSync(guidanceTarget, "stale");
    expect(() => generateCatalog(target, true)).toThrow("drifted");
    expect(readFileSync(guidanceTarget, "utf8")).toBe("stale");
  } finally {
    rmSync(directory, { recursive: true });
  }
});
