import type { Message } from "@/lib/types/http";
import type { MessageTextAnchor } from "@/lib/state/slices/comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";

const ANCHOR_CONTEXT_LENGTH = 80;

export function agentMessageCommentHighlightName(messageId: string) {
  return `agent-message-comment-${messageId.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
}

export type ResolvedMessageTextRange = { start: number; end: number };
export type MessageSelection = ResolvedMessageTextRange & {
  selectedText: string;
  rect: DOMRect;
};

export function createMessageTextAnchor(
  messageId: string,
  content: string,
  start: number,
  end: number,
): MessageTextAnchor {
  const safeStart = Math.max(0, Math.min(start, content.length));
  const safeEnd = Math.max(safeStart, Math.min(end, content.length));
  return {
    messageId,
    start: safeStart,
    end: safeEnd,
    selectedText: content.slice(safeStart, safeEnd),
    prefix: content.slice(Math.max(0, safeStart - ANCHOR_CONTEXT_LENGTH), safeStart),
    suffix: content.slice(safeEnd, safeEnd + ANCHOR_CONTEXT_LENGTH),
  };
}

function isValidRange(content: string, start: number, end: number, selectedText: string) {
  return start >= 0 && end > start && content.slice(start, end) === selectedText;
}

function findQuotedRange(
  anchor: MessageTextAnchor,
  content: string,
): ResolvedMessageTextRange | null {
  const quote = `${anchor.prefix}${anchor.selectedText}${anchor.suffix}`;
  const quoteIndex = content.indexOf(quote);
  if (quoteIndex !== -1) {
    const start = quoteIndex + anchor.prefix.length;
    return { start, end: start + anchor.selectedText.length };
  }

  const candidates: number[] = [];
  let candidate = content.indexOf(anchor.selectedText);
  while (candidate !== -1) {
    candidates.push(candidate);
    candidate = content.indexOf(anchor.selectedText, candidate + 1);
  }
  if (candidates.length === 0) return null;
  const prefix = anchor.prefix.slice(-32);
  const suffix = anchor.suffix.slice(0, 32);
  const best = candidates.reduce((current, next) => {
    const score = (index: number) => {
      const before = content.slice(Math.max(0, index - prefix.length), index);
      const after = content.slice(index + anchor.selectedText.length);
      return (
        (prefix && before.endsWith(prefix) ? 2 : 0) +
        (suffix && after.startsWith(suffix) ? 2 : 0) -
        Math.min(Math.abs(index - anchor.start), 1000) / 10000
      );
    };
    return score(next) > score(current) ? next : current;
  });
  return { start: best, end: best + anchor.selectedText.length };
}

export function resolveMessageTextAnchor(
  anchor: MessageTextAnchor,
  content: string,
): ResolvedMessageTextRange | null {
  if (!anchor.selectedText) return null;
  if (isValidRange(content, anchor.start, anchor.end, anchor.selectedText)) {
    return { start: anchor.start, end: anchor.end };
  }
  return findQuotedRange(anchor, content);
}

function boundaryOffset(root: HTMLElement, container: Node, offset: number): number | null {
  if (!root.contains(container)) return null;
  try {
    const range = document.createRange();
    range.selectNodeContents(root);
    range.setEnd(container, offset);
    return range.toString().length;
  } catch {
    return null;
  }
}

/** Convert a browser selection into offsets relative to one message body. */
export function getMessageSelection(
  root: HTMLElement,
  selection: Selection | null,
): MessageSelection | null {
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return null;
  const range = selection.getRangeAt(0);
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return null;
  const start = boundaryOffset(root, range.startContainer, range.startOffset);
  const end = boundaryOffset(root, range.endContainer, range.endOffset);
  if (start === null || end === null || end <= start) return null;
  const selectedText = range.toString();
  if (!selectedText.trim()) return null;
  return { start, end, selectedText, rect: range.getBoundingClientRect() };
}

function messageTextNodes(root: HTMLElement): Text[] {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes: Text[] = [];
  let node = walker.nextNode();
  while (node) {
    if (node.nodeValue) nodes.push(node as Text);
    node = walker.nextNode();
  }
  return nodes;
}

function domRangeForOffsets(root: HTMLElement, start: number, end: number): Range | null {
  let cursor = 0;
  let startBoundary: { node: Text; offset: number } | null = null;
  let endBoundary: { node: Text; offset: number } | null = null;
  for (const textNode of messageTextNodes(root)) {
    const textLength = textNode.data.length;
    const nodeStart = cursor;
    const nodeEnd = cursor + textLength;
    cursor = nodeEnd;
    if (!startBoundary && start >= nodeStart && start <= nodeEnd) {
      startBoundary = { node: textNode, offset: start - nodeStart };
    }
    if (end >= nodeStart && end <= nodeEnd) {
      endBoundary = { node: textNode, offset: end - nodeStart };
      break;
    }
  }
  if (!startBoundary || !endBoundary) return null;
  const range = document.createRange();
  range.setStart(startBoundary.node, startBoundary.offset);
  range.setEnd(endBoundary.node, endBoundary.offset);
  return range;
}

export type MessageCommentDecoration = {
  comment: AgentMessageComment;
  range: Range;
  left: number;
  top: number;
};

/** Resolve highlights without changing DOM nodes owned by React. */
export function getMessageCommentDecorations(
  root: HTMLElement,
  comments: AgentMessageComment[],
): MessageCommentDecoration[] {
  const content = root.textContent ?? "";
  const rootRect = root.getBoundingClientRect();
  const decorations: MessageCommentDecoration[] = [];
  for (const comment of comments) {
    const resolved = resolveMessageTextAnchor(comment.anchor, content);
    if (!resolved) continue;
    const range = domRangeForOffsets(root, resolved.start, resolved.end);
    if (!range) continue;
    const rects = Array.from(range.getClientRects());
    const rect = rects.at(-1) ?? range.getBoundingClientRect();
    decorations.push({
      comment,
      range,
      left: rect.right - rootRect.left + root.scrollLeft,
      top: rect.top - rootRect.top + root.scrollTop,
    });
  }
  return decorations;
}

export function isSelectableAgentMessage(
  message: Message,
  isTurnActive: boolean,
  isRawView: boolean,
): boolean {
  if (isTurnActive || isRawView) return false;
  if (message.author_type !== "agent") return false;
  if (message.type !== "message" && message.type !== "content") return false;
  if (!message.content.trim()) return false;
  return true;
}
