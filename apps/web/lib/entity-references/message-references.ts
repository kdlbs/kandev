import type { EntityReference } from "@/lib/types/entity-reference";

const IDENTITY_NAME = /^[a-z0-9][a-z0-9._:-]{0,127}$/;
const CONTROL_CHARACTER = /[\u0000-\u001f\u007f-\u009f]/u;
const UNPAIRED_SURROGATE = /[\uD800-\uDFFF]/u;
const MAX_IDENTITY_LENGTH = 512;
const MAX_TITLE_LENGTH = 500;
const MAX_KEY_LENGTH = 200;
const MAX_URL_LENGTH = 2048;
const REFERENCE_STRING_FIELDS = ["ref", "provider", "kind", "id", "title", "url", "scope"] as const;

function codePointLength(value: string): number {
  return Array.from(value).length;
}

function isBoundedIdentity(value: string): boolean {
  const length = codePointLength(value);
  return (
    length > 0 &&
    length <= MAX_IDENTITY_LENGTH &&
    !CONTROL_CHARACTER.test(value) &&
    !UNPAIRED_SURROGATE.test(value)
  );
}

function isNormalizedDisplayText(value: string, maxLength: number): boolean {
  return (
    value === value.trim().replace(/\s+/gu, " ") &&
    codePointLength(value) <= maxLength &&
    !UNPAIRED_SURROGATE.test(value)
  );
}

function queryEscape(value: string): string | null {
  try {
    return encodeURIComponent(value)
      .replace(/[!'()*]/g, (character) => `%${character.charCodeAt(0).toString(16).toUpperCase()}`)
      .replace(/%20/g, "+");
  } catch {
    return null;
  }
}

function canonicalRef(reference: EntityReference): string | null {
  const fields = [reference.provider, reference.kind, reference.scope, reference.id];
  const encoded = fields.map(queryEscape);
  if (encoded.some((field) => field === null)) return null;
  return `mention:v1:${encoded.join(":")}`;
}

function hasSafeURL(reference: EntityReference): boolean {
  if (
    reference.url.length === 0 ||
    codePointLength(reference.url) > MAX_URL_LENGTH ||
    UNPAIRED_SURROGATE.test(reference.url)
  ) {
    return false;
  }
  if (reference.provider === "kandev" && reference.kind === "task") {
    return reference.url === `/t/${encodeURIComponent(reference.id)}`;
  }
  try {
    const parsed = new URL(reference.url);
    return (
      (parsed.protocol === "http:" || parsed.protocol === "https:") &&
      parsed.hostname.length > 0 &&
      parsed.username === "" &&
      parsed.password === ""
    );
  } catch {
    return false;
  }
}

function hasValidIdentityFields(reference: EntityReference): boolean {
  return (
    IDENTITY_NAME.test(reference.provider) &&
    IDENTITY_NAME.test(reference.kind) &&
    isBoundedIdentity(reference.id) &&
    isBoundedIdentity(reference.scope) &&
    reference.ref === canonicalRef(reference)
  );
}

function hasValidDisplayFields(reference: EntityReference): boolean {
  return (
    reference.title.length > 0 &&
    isNormalizedDisplayText(reference.title, MAX_TITLE_LENGTH) &&
    isNormalizedDisplayText(reference.key ?? "", MAX_KEY_LENGTH)
  );
}

export function isEntityReference(value: unknown): value is EntityReference {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const reference = value as Record<string, unknown>;
  if (reference.version !== 1) return false;
  if (!REFERENCE_STRING_FIELDS.every((field) => typeof reference[field] === "string")) return false;
  if (reference.key !== undefined && typeof reference.key !== "string") return false;
  const typed = reference as EntityReference;
  return hasValidIdentityFields(typed) && hasValidDisplayFields(typed) && hasSafeURL(typed);
}

export function normalizeEntityReferences(value: unknown): EntityReference[] {
  if (!Array.isArray(value)) return [];
  const result: EntityReference[] = [];
  const seen = new Set<string>();
  for (const candidate of value) {
    if (!isEntityReference(candidate) || seen.has(candidate.ref)) continue;
    seen.add(candidate.ref);
    result.push(candidate);
  }
  return result;
}

export function entityReferencesFromMetadata(metadata: unknown): EntityReference[] {
  if (!metadata || typeof metadata !== "object" || Array.isArray(metadata)) return [];
  return normalizeEntityReferences((metadata as Record<string, unknown>).entity_references);
}

function escapeMarkdownLabel(value: string): string {
  return value.replace(/([\\[\]])/g, "\\$1");
}

export function entityReferenceHref(reference: EntityReference): string {
  const value = reference.url;
  return encodeURI(value).replace(/\(/g, "%28").replace(/\)/g, "%29");
}

export function entityReferenceLabel(reference: EntityReference): string {
  return `#${reference.key || reference.title}`;
}

export function entityReferenceMarkdown(reference: EntityReference): string {
  return `[${escapeMarkdownLabel(entityReferenceLabel(reference))}](${entityReferenceHref(reference)})`;
}

export function matchEntityReferenceLink(
  references: readonly EntityReference[],
  label: string,
  href: string | undefined,
): EntityReference | null {
  if (!href) return null;
  let match: EntityReference | null = null;
  for (const reference of normalizeEntityReferences(references)) {
    if (entityReferenceLabel(reference) !== label || entityReferenceHref(reference) !== href) {
      continue;
    }
    if (match) return null;
    match = reference;
  }
  return match;
}

type Fence = { marker: "`" | "~"; length: number };

function fenceAtLineStart(line: string): Fence | null {
  const match = /^ {0,3}(`{3,}|~{3,})/u.exec(line);
  if (!match) return null;
  const run = match[1];
  return { marker: run[0] as Fence["marker"], length: run.length };
}

function closesFence(line: string, fence: Fence): boolean {
  const trimmed = line.replace(/^ {0,3}/u, "").trimEnd();
  if (!trimmed.startsWith(fence.marker)) return false;
  let runLength = 0;
  while (trimmed[runLength] === fence.marker) runLength++;
  return runLength >= fence.length && trimmed.length === runLength;
}

function markRange(mask: Uint8Array, start: number, end: number): void {
  mask.fill(1, start, end);
}

function backtickRunLength(line: string, start: number): number {
  let end = start;
  while (line[end] === "`") end++;
  return end - start;
}

function findClosingBackticks(line: string, start: number, runLength: number): number {
  let cursor = start;
  while (cursor < line.length) {
    const next = line.indexOf("`", cursor);
    if (next === -1) return -1;
    const candidateLength = backtickRunLength(line, next);
    if (candidateLength === runLength) return next;
    cursor = next + candidateLength;
  }
  return -1;
}

function markInlineCode(line: string, lineStart: number, mask: Uint8Array): void {
  let cursor = 0;
  while (cursor < line.length) {
    const opener = line.indexOf("`", cursor);
    if (opener === -1) return;
    const runLength = backtickRunLength(line, opener);
    const closer = findClosingBackticks(line, opener + runLength, runLength);
    if (closer === -1) {
      cursor = opener + runLength;
      continue;
    }
    markRange(mask, lineStart + opener, lineStart + closer + runLength);
    cursor = closer + runLength;
  }
}

function markdownCodeMask(content: string): Uint8Array {
  const mask = new Uint8Array(content.length);
  let fence: Fence | null = null;
  let lineStart = 0;
  while (lineStart < content.length) {
    const newline = content.indexOf("\n", lineStart);
    const lineEnd = newline === -1 ? content.length : newline;
    const line = content.slice(lineStart, lineEnd);
    if (fence) {
      markRange(mask, lineStart, lineEnd);
      if (closesFence(line, fence)) fence = null;
    } else {
      const opener = fenceAtLineStart(line);
      if (opener) {
        markRange(mask, lineStart, lineEnd);
        fence = opener;
      } else if (/^(?: {4}|\t)/u.test(line)) {
        markRange(mask, lineStart, lineEnd);
      } else {
        markInlineCode(line, lineStart, mask);
      }
    }
    lineStart = lineEnd + 1;
  }
  return mask;
}

function isEscaped(content: string, index: number): boolean {
  let backslashes = 0;
  for (let cursor = index - 1; cursor >= 0 && content[cursor] === "\\"; cursor--) backslashes++;
  return backslashes % 2 === 1;
}

function findUsableOccurrence(
  content: string,
  markdown: string,
  cursor: number,
  codeMask: Uint8Array,
): number {
  let index = content.indexOf(markdown, cursor);
  while (index !== -1) {
    const isImage = index > 0 && content[index - 1] === "!" && !isEscaped(content, index - 1);
    if (!codeMask[index] && !isEscaped(content, index) && !isImage) return index;
    index = content.indexOf(markdown, index + markdown.length);
  }
  return -1;
}

export function survivingEntityReferences(
  content: string,
  references: readonly EntityReference[],
): EntityReference[] {
  const pending = normalizeEntityReferences(references).map((reference) => ({
    reference,
    markdown: entityReferenceMarkdown(reference),
  }));
  const result: EntityReference[] = [];
  const codeMask = markdownCodeMask(content);
  let cursor = 0;
  while (pending.length > 0) {
    let nextIndex = -1;
    let pendingIndex = -1;
    for (let index = 0; index < pending.length; index++) {
      const candidateIndex = findUsableOccurrence(
        content,
        pending[index].markdown,
        cursor,
        codeMask,
      );
      if (candidateIndex !== -1 && (nextIndex === -1 || candidateIndex < nextIndex)) {
        nextIndex = candidateIndex;
        pendingIndex = index;
      }
    }
    if (pendingIndex === -1) break;
    const [match] = pending.splice(pendingIndex, 1);
    result.push(match.reference);
    cursor = nextIndex + match.markdown.length;
  }
  return result;
}
