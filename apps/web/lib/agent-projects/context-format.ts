import { isMap, isScalar, parseDocument } from "yaml";

// i18n-exempt: closed domain discriminants are translated by the context panel.
export type ProjectContextDocumentKind = "text" | "markdown" | "concept" | "index" | "log";

// i18n-exempt: closed diagnostic codes are translated by the context panel.
export type ProjectContextFormatDiagnostic =
  | "header_unclosed"
  | "header_too_large"
  | "invalid_yaml"
  | "duplicate_key"
  | "not_mapping"
  | "concept_type_missing"
  | "reserved_file_type"
  | "unsupported_version";

export interface ProjectContextFormatInspection {
  kind: ProjectContextDocumentKind;
  diagnostics: ProjectContextFormatDiagnostic[];
}

const MAX_FRONTMATTER_BYTES = 64 * 1024;

export function inspectProjectContextFormat(
  path: string,
  draft: string,
): ProjectContextFormatInspection {
  const pathInfo = inspectMarkdownPath(path);
  if (!pathInfo) return { kind: "text", diagnostics: [] };
  const content = draft.charCodeAt(0) === 0xfeff ? draft.slice(1) : draft;
  const frontmatter = inspectFrontmatter(content);
  if (frontmatter.type === "none") return { kind: pathInfo.kind, diagnostics: [] };
  if (frontmatter.type === "diagnostic") {
    return { kind: pathInfo.headerKind, diagnostics: frontmatter.diagnostics };
  }
  return inspectMetadata(frontmatter.document, pathInfo);
}

interface MarkdownPathInfo {
  kind: ProjectContextDocumentKind;
  headerKind: ProjectContextDocumentKind;
  isReserved: boolean;
  isRootIndex: boolean;
}

function inspectMarkdownPath(path: string): MarkdownPathInfo | null {
  const normalizedPath = path.replace(/\\/g, "/").replace(/^\/+/, "");
  const pathParts = normalizedPath.split("/").filter(Boolean);
  const fileName = pathParts.at(-1) ?? "";
  if (!/\.(?:md|markdown)$/i.test(fileName)) return null;

  const isIndex = fileName === "index.md";
  const isLog = fileName === "log.md";
  let kind: ProjectContextDocumentKind = "markdown";
  if (isIndex) kind = "index";
  else if (isLog) kind = "log";

  const isReserved = isIndex || isLog;
  return {
    kind,
    headerKind: isReserved ? kind : "concept",
    isReserved,
    isRootIndex: isIndex && pathParts.length === 1,
  };
}

type FrontmatterInspection =
  | { type: "none" }
  | { type: "diagnostic"; diagnostics: ProjectContextFormatDiagnostic[] }
  | { type: "document"; document: ReturnType<typeof parseDocument> };

function inspectFrontmatter(content: string): FrontmatterInspection {
  const bounded = takeUTF8Prefix(content, MAX_FRONTMATTER_BYTES);
  const lines = bounded.text.replace(/\r\n/g, "\n").split("\n");
  if (stripLineWhitespace(lines[0] ?? "") !== "---") return { type: "none" };

  const closingLine = findClosingLine(lines, bounded, content);
  if (closingLine < 0) {
    const diagnostic = bounded.exceeded ? "header_too_large" : "header_unclosed";
    return { type: "diagnostic", diagnostics: [diagnostic] };
  }

  return parseFrontmatterYaml(lines.slice(1, closingLine).join("\n"));
}

function findClosingLine(
  lines: string[],
  bounded: { text: string; exceeded: boolean },
  content: string,
): number {
  return lines.findIndex((line, index) => {
    if (index === 0 || stripLineWhitespace(line) !== "---") return false;
    if (bounded.exceeded && index === lines.length - 1) {
      return hasLineEndingAt(content, bounded.text.length);
    }
    return true;
  });
}

function parseFrontmatterYaml(header: string): FrontmatterInspection {
  let document: ReturnType<typeof parseDocument>;
  try {
    document = parseDocument(header, { uniqueKeys: true });
  } catch {
    return { type: "diagnostic", diagnostics: ["invalid_yaml"] };
  }

  if (document.errors.length > 0) {
    const diagnostics: ProjectContextFormatDiagnostic[] = [];
    if (document.errors.some((error) => error.code === "DUPLICATE_KEY")) {
      diagnostics.push("duplicate_key");
    }
    if (document.errors.some((error) => error.code !== "DUPLICATE_KEY")) {
      diagnostics.push("invalid_yaml");
    }
    return { type: "diagnostic", diagnostics };
  }

  if (!isMap(document.contents)) {
    return { type: "diagnostic", diagnostics: ["not_mapping"] };
  }
  return { type: "document", document };
}

function inspectMetadata(
  document: ReturnType<typeof parseDocument>,
  pathInfo: MarkdownPathInfo,
): ProjectContextFormatInspection {
  const diagnostics: ProjectContextFormatDiagnostic[] = [];
  if (pathInfo.isReserved) {
    if (document.has("type")) diagnostics.push("reserved_file_type");
    if (pathInfo.isRootIndex && !hasSupportedVersion(document)) {
      diagnostics.push("unsupported_version");
    }
    return { kind: pathInfo.kind, diagnostics };
  }

  const type = scalarValue(document.get("type", true));
  if (typeof type !== "string" || type.trim() === "") {
    diagnostics.push("concept_type_missing");
  }
  return { kind: "concept", diagnostics };
}

function hasSupportedVersion(document: ReturnType<typeof parseDocument>): boolean {
  if (!document.has("okf_version")) return true;
  const version = scalarValue(document.get("okf_version", true));
  return (
    (typeof version === "string" && version === "0.2") ||
    (typeof version === "number" && version === 0.2)
  );
}

function stripLineWhitespace(line: string): string {
  return line.replace(/\r$/, "").replace(/[ \t]+$/, "");
}

function hasLineEndingAt(value: string, index: number): boolean {
  const next = value[index];
  return next === "\n" || (next === "\r" && value[index + 1] === "\n");
}

function scalarValue(value: unknown): unknown {
  return isScalar(value) ? value.value : undefined;
}

function takeUTF8Prefix(value: string, limit: number): { text: string; exceeded: boolean } {
  let end = 0;
  let bytes = 0;
  while (end < value.length) {
    const codePoint = value.codePointAt(end);
    if (codePoint === undefined) break;
    let characterBytes: number;
    if (codePoint <= 0x7f) characterBytes = 1;
    else if (codePoint <= 0x7ff) characterBytes = 2;
    else if (codePoint <= 0xffff) characterBytes = 3;
    else characterBytes = 4;
    if (bytes + characterBytes > limit) return { text: value.slice(0, end), exceeded: true };
    bytes += characterBytes;
    end += codePoint > 0xffff ? 2 : 1;
  }
  return { text: value, exceeded: false };
}
