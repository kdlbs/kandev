import { describe, expect, it } from "vitest";
import { inspectProjectContextFormat } from "./context-format";

const CONCEPT_TYPE_MISSING = "concept_type_missing";
const CONCEPT_FILE = "decision.md";

describe("inspectProjectContextFormat", () => {
  it("treats ordinary text as text without applying Markdown metadata rules", () => {
    expect(inspectProjectContextFormat("notes.txt", "---\ntitle: Plain text\n---\nbody\n")).toEqual(
      { kind: "text", diagnostics: [] },
    );
  });

  it("recognizes plain Markdown without inspecting later separators or fenced examples", () => {
    const draft = ["# Guide", "", "---", "", "```md", "---", "type: Example", "---", "```"].join(
      "\n",
    );

    expect(inspectProjectContextFormat("guide.md", draft)).toEqual({
      kind: "markdown",
      diagnostics: [],
    });
    expect(draft).toContain("type: Example");
  });

  it("recognizes root indexes and the supported string or numeric version", () => {
    expect(inspectProjectContextFormat("index.md", "# Project context\n")).toEqual({
      kind: "index",
      diagnostics: [],
    });
    expect(inspectProjectContextFormat("index.md", '---\nokf_version: "0.2"\n---\n')).toEqual({
      kind: "index",
      diagnostics: [],
    });
    expect(inspectProjectContextFormat("index.md", "---\nokf_version: 0.2\n---\n")).toEqual({
      kind: "index",
      diagnostics: [],
    });
  });

  it.each(["0.3", '"0.1"', "true", "{ major: 0, minor: 2 }"])(
    "reports an unsupported root index version %s without blocking inspection",
    (version) => {
      expect(
        inspectProjectContextFormat("index.md", `---\nokf_version: ${version}\n---\n`),
      ).toEqual({ kind: "index", diagnostics: ["unsupported_version"] });
    },
  );

  it("accepts custom concept types and unfamiliar optional metadata", () => {
    const draft = [
      "---",
      "type: Architecture",
      "title: Runtime boundaries",
      "source_system: internal",
      "custom_field:",
      "  enabled: true",
      "---",
      "# Runtime boundaries",
      "",
      "<!-- Keep this comment exactly as authored. -->",
    ].join("\n");

    expect(inspectProjectContextFormat("architecture.md", draft)).toEqual({
      kind: "concept",
      diagnostics: [],
    });
    expect(draft).toContain("<!-- Keep this comment exactly as authored. -->");
  });

  it.each(["---\ntitle: Untyped\n---\n", '---\ntype: "  "\n---\n', "---\ntype: 7\n---\n"])(
    "advises when a concept type is absent or not a nonempty string",
    (draft) => {
      expect(inspectProjectContextFormat("notes.md", draft)).toEqual({
        kind: "concept",
        diagnostics: [CONCEPT_TYPE_MISSING],
      });
    },
  );

  it.each([
    ["docs/index.md", "index", "---\ntype: Index\n---\n"],
    ["log.md", "log", "---\ntype: Update Log\n---\n"],
  ] as const)("keeps reserved file %s out of the concept type rule", (path, kind, draft) => {
    expect(inspectProjectContextFormat(path, draft)).toEqual({
      kind,
      diagnostics: ["reserved_file_type"],
    });
  });
});

describe("inspectProjectContextFormat header diagnostics", () => {
  it.each([
    ["invalid YAML", "---\ntype: [\n---\n", "invalid_yaml"],
    ["duplicate keys", "---\ntype: Decision\ntype: Reference\n---\n", "duplicate_key"],
    ["a non-mapping header", "---\nplain scalar\n---\n", "not_mapping"],
  ] as const)("reports %s as advisory structure feedback", (_name, draft, diagnostic) => {
    expect(inspectProjectContextFormat(CONCEPT_FILE, draft)).toEqual({
      kind: "concept",
      diagnostics: [diagnostic],
    });
  });

  it("inspects a BOM and CRLF header without changing the current draft", () => {
    const draft = "\uFEFF---\r\ntype: Decision\r\ntitle: Decision log\r\n---\r\n# Decision\r\n";
    const before = draft;

    expect(inspectProjectContextFormat(CONCEPT_FILE, draft)).toEqual({
      kind: "concept",
      diagnostics: [],
    });
    expect(draft).toBe(before);
  });

  it.each([
    ["an unclosed header", `---\ntype: Decision\ntitle: open\n`, "header_unclosed"],
    ["an oversized header", `---\n${"x".repeat(65_536)}\n---\n`, "header_too_large"],
  ] as const)("reports %s while leaving the draft available", (_name, draft, diagnostic) => {
    expect(inspectProjectContextFormat(CONCEPT_FILE, draft)).toEqual({
      kind: "concept",
      diagnostics: [diagnostic],
    });
    expect(draft.length).toBeGreaterThan(0);
  });

  it("does not treat a truncated closing line as a delimiter", () => {
    const prefix = "---\ntype: Decision\n#";
    const draft = `${prefix}${"x".repeat(65_536 - prefix.length - 4)}\n----\n`;

    expect(inspectProjectContextFormat(CONCEPT_FILE, draft)).toEqual({
      kind: "concept",
      diagnostics: ["header_too_large"],
    });
  });

  it("accepts a complete closing delimiter at the inspection boundary", () => {
    const prefix = "---\ntype: Decision\n#";
    const draft = `${prefix}${"x".repeat(65_536 - prefix.length - 4)}\n---\n`;

    expect(inspectProjectContextFormat(CONCEPT_FILE, draft)).toEqual({
      kind: "concept",
      diagnostics: [],
    });
  });
});
