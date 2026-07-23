import type React from "react";
import type { EntityReference } from "@/lib/types/entity-reference";
import { isEntityReference } from "@/lib/entity-references/message-references";
import { formatSlashCommandLabel, normalizeSlashCommandName } from "./tiptap-slash-command-utils";
import type { SlashCommand } from "./slash-command-types";

// ── JSON node types ─────────────────────────────────────────────────

export type JSONNode = {
  type?: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: Array<{ type: string }>;
  content?: JSONNode[];
};

export type EditorContentNode = {
  type: string;
  text?: string;
  attrs?: Record<string, unknown>;
  content?: EditorContentNode[];
};

export function isEntityReferenceTriggerAllowed(args: {
  textBeforeTrigger: string;
  parentType: string;
  hasCodeMark: boolean;
}): boolean {
  if (args.parentType === "codeBlock" || args.hasCodeMark) return false;
  return args.textBeforeTrigger === "" || /\s$/u.test(args.textBeforeTrigger);
}

function entityReferenceFromAttrs(attrs: Record<string, unknown> | undefined) {
  if (!attrs) return null;
  const candidate = attrs.key === null ? { ...attrs, key: undefined } : attrs;
  return isEntityReference(candidate) ? candidate : null;
}

export function extractEntityReferences(doc: JSONNode): EntityReference[] {
  const references: EntityReference[] = [];
  const seen = new Set<string>();
  const visit = (node: JSONNode) => {
    if (node.type === "entityReference") {
      const reference = entityReferenceFromAttrs(node.attrs);
      if (reference && !seen.has(reference.ref)) {
        seen.add(reference.ref);
        references.push(reference);
      }
    }
    node.content?.forEach(visit);
  };
  visit(doc);
  return references;
}

// ── Serialization ───────────────────────────────────────────────────

function serializeInline(nodes: JSONNode[]): string {
  return nodes
    .map((n) => {
      if (n.type === "hardBreak") return "\n";
      if (n.type === "contextMention") {
        return n.attrs?.label ? `@${n.attrs.label}` : "";
      }
      if (n.type === "entityReference") {
        return formatEntityReferenceMarkdown(n.attrs as Partial<EntityReference>);
      }
      if (n.type === "slashCommand") {
        return formatSlashCommandLabel(n.attrs);
      }
      const text = n.text ?? "";
      if (n.marks?.some((m) => m.type === "code")) {
        return "`" + text + "`";
      }
      return text;
    })
    .join("");
}

function escapeMarkdownLabel(value: string): string {
  return value.replace(/([\\[\]])/g, "\\$1");
}

function encodeMarkdownUrl(value: string): string {
  return encodeURI(value).replace(/\(/g, "%28").replace(/\)/g, "%29");
}

export function formatEntityReferenceMarkdown(reference: Partial<EntityReference>): string {
  const label = String(reference.key || reference.title || "");
  const url = String(reference.url || "");
  if (!label || !url) return "";
  return `[#${escapeMarkdownLabel(label)}](${encodeMarkdownUrl(url)})`;
}

function serializeNode(node: JSONNode): string {
  switch (node.type) {
    case "paragraph":
      return serializeInline(node.content ?? []);
    case "codeBlock": {
      const lang = (node.attrs?.language as string) || "";
      const text = serializeInline(node.content ?? []);
      return "```" + lang + "\n" + text + "\n```";
    }
    case "hardBreak":
      return "\n";
    default:
      // Unknown block — try to serialize children
      if (node.content) return node.content.map(serializeNode).join("\n");
      return node.text ?? "";
  }
}

/**
 * Serialize TipTap editor content to markdown-like text.
 * Preserves inline `code`, ```code blocks```, and @mention labels.
 */
export function getMarkdownText(editor: { getJSON: () => { content?: JSONNode[] } }): string {
  const doc = editor.getJSON();
  if (!doc.content) return "";
  return doc.content.map(serializeNode).join("\n");
}

// ── HTML escaping ───────────────────────────────────────────────────

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

/** Convert plain text with newlines to HTML paragraphs for TipTap */
export function textToHtml(text: string): string {
  const lines = text.split("\n");
  return lines.map((line) => `<p>${escapeHtml(line) || "<br>"}</p>`).join("");
}

function slashCommandName(command: SlashCommand): string {
  return normalizeSlashCommandName(command.agentCommandName || command.label);
}

function slashCommandAttrs(command: SlashCommand): Record<string, unknown> {
  const name = slashCommandName(command);
  return {
    id: command.id,
    label: `/${name}`,
    commandName: name,
    description: command.description,
  };
}

function slashCommandMap(commands: readonly SlashCommand[]): Map<string, SlashCommand> {
  const map = new Map<string, SlashCommand>();
  for (const command of commands) {
    const name = slashCommandName(command);
    if (name) map.set(name, command);
  }
  return map;
}

function slashTextToNodes(line: string, commands: Map<string, SlashCommand>): EditorContentNode[] {
  const nodes: EditorContentNode[] = [];
  const tokenPattern = /\/\S+/g;
  let cursor = 0;
  for (const match of line.matchAll(tokenPattern)) {
    const index = match.index ?? 0;
    const token = match[0];
    const command = commands.get(normalizeSlashCommandName(token));
    if (!command) continue;
    if (index > cursor) {
      nodes.push({ type: "text", text: line.slice(cursor, index) });
    }
    nodes.push({ type: "slashCommand", attrs: slashCommandAttrs(command) });
    cursor = index + token.length;
  }
  if (cursor < line.length) {
    nodes.push({ type: "text", text: line.slice(cursor) });
  }
  return nodes;
}

function entityReferenceTokens(references: readonly EntityReference[]) {
  return references
    .map((reference) => ({ reference, markdown: formatEntityReferenceMarkdown(reference) }))
    .filter((token) => token.markdown.length > 0);
}

function textLineToNodes(
  line: string,
  commands: Map<string, SlashCommand>,
  references: readonly EntityReference[],
): EditorContentNode[] {
  const tokens = entityReferenceTokens(references);
  if (tokens.length === 0) return slashTextToNodes(line, commands);
  const nodes: EditorContentNode[] = [];
  let cursor = 0;
  while (cursor < line.length) {
    let match: { index: number; markdown: string; reference: EntityReference } | null = null;
    for (const token of tokens) {
      const index = line.indexOf(token.markdown, cursor);
      if (index !== -1 && (!match || index < match.index)) match = { ...token, index };
    }
    if (!match) {
      nodes.push(...slashTextToNodes(line.slice(cursor), commands));
      break;
    }
    if (match.index > cursor) {
      nodes.push(...slashTextToNodes(line.slice(cursor, match.index), commands));
    }
    nodes.push({ type: "entityReference", attrs: { ...match.reference } });
    cursor = match.index + match.markdown.length;
  }
  return nodes;
}

export function textToEditorContent(
  text: string,
  slashCommands: readonly SlashCommand[] = [],
  entityReferences: readonly EntityReference[] = [],
): EditorContentNode {
  const commands = slashCommandMap(slashCommands);
  const content: EditorContentNode[] = [];
  let inCodeFence = false;
  for (const line of text.split("\n")) {
    const isFenceLine = line.trimStart().startsWith("```");
    let nodes: EditorContentNode[];
    if (inCodeFence || isFenceLine) {
      nodes = line ? [{ type: "text", text: line }] : [];
    } else {
      nodes = textLineToNodes(line, commands, entityReferences);
    }
    content.push(nodes.length > 0 ? { type: "paragraph", content: nodes } : { type: "paragraph" });
    if (isFenceLine) inCodeFence = !inCodeFence;
  }
  return {
    type: "doc",
    content,
  };
}

// ── Code fence parsing ──────────────────────────────────────────────

export type FenceSegment =
  | { type: "text"; text: string }
  | { type: "code"; text: string; language: string | null };

/** Parse text containing markdown ``` fences into text/code segments. */
export function parseCodeFences(text: string): FenceSegment[] {
  const lines = text.split("\n");
  const segments: FenceSegment[] = [];
  let currentType: "text" | "code" = "text";
  let currentLines: string[] = [];
  let currentLang: string | null = null;

  for (const line of lines) {
    if (line.trimStart().startsWith("```")) {
      if (currentType === "text") {
        if (currentLines.length > 0) {
          segments.push({ type: "text", text: currentLines.join("\n") });
        }
        currentLang = line.trimStart().slice(3).trim() || null;
        currentLines = [];
        currentType = "code";
      } else {
        segments.push({ type: "code", text: currentLines.join("\n"), language: currentLang });
        currentLines = [];
        currentLang = null;
        currentType = "text";
      }
    } else {
      currentLines.push(line);
    }
  }

  if (currentLines.length > 0) {
    if (currentType === "code") {
      segments.push({ type: "code", text: currentLines.join("\n"), language: currentLang });
    } else {
      segments.push({ type: "text", text: currentLines.join("\n") });
    }
  }

  return segments;
}

// ── Paste handler ───────────────────────────────────────────────────

function extractFiles(items: DataTransferItemList): File[] {
  const files: File[] = [];
  for (const item of items) {
    if (item.kind === "file") {
      const file = item.getAsFile();
      if (file) files.push(file);
    }
  }
  return files;
}

function segmentToNodes(
  seg: FenceSegment,
  schema: import("@tiptap/pm/model").Schema,
): import("@tiptap/pm/model").Node[] {
  if (seg.type === "code") {
    return [
      schema.nodes.codeBlock.create(
        seg.language ? { language: seg.language } : null,
        seg.text ? schema.text(seg.text) : undefined,
      ),
    ];
  }
  const trimmed = seg.text.trim();
  if (!trimmed) return [];
  return trimmed
    .split("\n")
    .map((line) => schema.nodes.paragraph.create(null, line ? schema.text(line) : undefined));
}

function insertCodeFenceNodes(
  view: import("@tiptap/pm/view").EditorView,
  segments: FenceSegment[],
): void {
  const { schema } = view.state;
  const nodes = segments.flatMap((seg) => segmentToNodes(seg, schema));
  if (nodes.length > 0) {
    const { from, to } = view.state.selection;
    view.dispatch(view.state.tr.replaceWith(from, to, nodes));
  }
}

export function handleEditorPaste(
  view: import("@tiptap/pm/view").EditorView,
  event: ClipboardEvent,
  onImagePasteRef: React.RefObject<((files: File[]) => void) | undefined>,
): boolean {
  // 1. File paste (images and other files)
  const items = event.clipboardData?.items;
  if (items) {
    const files = extractFiles(items);
    if (files.length > 0) {
      event.preventDefault();
      onImagePasteRef.current?.(files);
      return true;
    }
  }

  // 2. Markdown code fence paste
  const text = event.clipboardData?.getData("text/plain");
  if (text && text.includes("```")) {
    const segments = parseCodeFences(text);
    if (segments.some((s) => s.type === "code")) {
      event.preventDefault();
      insertCodeFenceNodes(view, segments);
      return true;
    }
  }

  return false;
}
