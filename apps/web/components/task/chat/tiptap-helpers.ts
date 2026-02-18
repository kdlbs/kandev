import type React from 'react';

// ── JSON node types ─────────────────────────────────────────────────

export type JSONNode = {
  type?: string;
  text?: string;
  attrs?: Record<string, unknown>;
  marks?: Array<{ type: string }>;
  content?: JSONNode[];
};

// ── Serialization ───────────────────────────────────────────────────

function serializeInline(nodes: JSONNode[]): string {
  return nodes.map((n) => {
    if (n.type === 'hardBreak') return '\n';
    if (n.type === 'contextMention') {
      return n.attrs?.label ? `@${n.attrs.label}` : '';
    }
    const text = n.text ?? '';
    if (n.marks?.some((m) => m.type === 'code')) {
      return '`' + text + '`';
    }
    return text;
  }).join('');
}

function serializeNode(node: JSONNode): string {
  switch (node.type) {
    case 'paragraph':
      return serializeInline(node.content ?? []);
    case 'codeBlock': {
      const lang = (node.attrs?.language as string) || '';
      const text = serializeInline(node.content ?? []);
      return '```' + lang + '\n' + text + '\n```';
    }
    case 'hardBreak':
      return '\n';
    default:
      // Unknown block — try to serialize children
      if (node.content) return node.content.map(serializeNode).join('\n');
      return node.text ?? '';
  }
}

/**
 * Serialize TipTap editor content to markdown-like text.
 * Preserves inline `code`, ```code blocks```, and @mention labels.
 */
export function getMarkdownText(editor: { getJSON: () => { content?: JSONNode[] } }): string {
  const doc = editor.getJSON();
  if (!doc.content) return '';
  return doc.content.map(serializeNode).join('\n');
}

// ── HTML escaping ───────────────────────────────────────────────────

export function escapeHtml(str: string): string {
  return str
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

// ── Code fence parsing ──────────────────────────────────────────────

export type FenceSegment =
  | { type: 'text'; text: string }
  | { type: 'code'; text: string; language: string | null };

/** Parse text containing markdown ``` fences into text/code segments. */
export function parseCodeFences(text: string): FenceSegment[] {
  const lines = text.split('\n');
  const segments: FenceSegment[] = [];
  let currentType: 'text' | 'code' = 'text';
  let currentLines: string[] = [];
  let currentLang: string | null = null;

  for (const line of lines) {
    if (line.trimStart().startsWith('```')) {
      if (currentType === 'text') {
        if (currentLines.length > 0) {
          segments.push({ type: 'text', text: currentLines.join('\n') });
        }
        currentLang = line.trimStart().slice(3).trim() || null;
        currentLines = [];
        currentType = 'code';
      } else {
        segments.push({ type: 'code', text: currentLines.join('\n'), language: currentLang });
        currentLines = [];
        currentLang = null;
        currentType = 'text';
      }
    } else {
      currentLines.push(line);
    }
  }

  if (currentLines.length > 0) {
    if (currentType === 'code') {
      segments.push({ type: 'code', text: currentLines.join('\n'), language: currentLang });
    } else {
      segments.push({ type: 'text', text: currentLines.join('\n') });
    }
  }

  return segments;
}

// ── Paste handler ───────────────────────────────────────────────────

function extractImageFiles(items: DataTransferItemList): File[] {
  const files: File[] = [];
  for (const item of items) {
    if (item.type.startsWith('image/')) {
      const file = item.getAsFile();
      if (file) files.push(file);
    }
  }
  return files;
}

function segmentToNodes(
  seg: FenceSegment,
  schema: import('@tiptap/pm/model').Schema,
): import('@tiptap/pm/model').Node[] {
  if (seg.type === 'code') {
    return [
      schema.nodes.codeBlock.create(
        seg.language ? { language: seg.language } : null,
        seg.text ? schema.text(seg.text) : undefined,
      ),
    ];
  }
  const trimmed = seg.text.trim();
  if (!trimmed) return [];
  return trimmed.split('\n').map((line) =>
    schema.nodes.paragraph.create(null, line ? schema.text(line) : undefined),
  );
}

function insertCodeFenceNodes(
  view: import('@tiptap/pm/view').EditorView,
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
  view: import('@tiptap/pm/view').EditorView,
  event: ClipboardEvent,
  onImagePasteRef: React.RefObject<((files: File[]) => void) | undefined>,
): boolean {
  // 1. Image paste
  const items = event.clipboardData?.items;
  if (items) {
    const imageFiles = extractImageFiles(items);
    if (imageFiles.length > 0) {
      event.preventDefault();
      onImagePasteRef.current?.(imageFiles);
      return true;
    }
  }

  // 2. Markdown code fence paste
  const text = event.clipboardData?.getData('text/plain');
  if (text && text.includes('```')) {
    const segments = parseCodeFences(text);
    if (segments.some((s) => s.type === 'code')) {
      event.preventDefault();
      insertCodeFenceNodes(view, segments);
      return true;
    }
  }

  return false;
}
