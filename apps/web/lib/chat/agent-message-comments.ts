import type { Message } from "@/lib/types/http";
import type { MessageTextAnchor } from "@/lib/state/slices/comments";
import type { AgentMessageComment } from "@/lib/state/slices/comments";

const ANCHOR_CONTEXT_LENGTH = 80;

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

function createCommentBadge(commentId: string): HTMLSpanElement {
  const badge = document.createElement("span");
  badge.className = "comment-badge agent-message-comment-badge";
  badge.dataset.agentMessageCommentId = commentId;
  badge.dataset.commentId = commentId;
  badge.setAttribute("role", "button");
  badge.setAttribute("tabindex", "0");
  badge.setAttribute("aria-label", "Edit comment");
  badge.innerHTML =
    '<svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
    'stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
    '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>';
  return badge;
}

function wrapTextRange(
  root: HTMLElement,
  start: number,
  end: number,
  commentId: string,
): HTMLElement | null {
  let cursor = 0;
  let lastMark: HTMLElement | null = null;
  for (const textNode of messageTextNodes(root)) {
    const textLength = textNode.data.length;
    const nodeStart = cursor;
    const nodeEnd = cursor + textLength;
    cursor = nodeEnd;
    const overlapStart = Math.max(start, nodeStart) - nodeStart;
    const overlapEnd = Math.min(end, nodeEnd) - nodeStart;
    if (overlapEnd <= overlapStart) continue;

    let selectedNode = textNode;
    if (overlapEnd < selectedNode.data.length) selectedNode.splitText(overlapEnd);
    if (overlapStart > 0) selectedNode = selectedNode.splitText(overlapStart);
    const mark = document.createElement("mark");
    mark.dataset.agentMessageCommentId = commentId;
    mark.dataset.commentId = commentId;
    mark.className = "comment-highlight agent-message-comment-highlight";
    selectedNode.parentNode?.replaceChild(mark, selectedNode);
    mark.appendChild(selectedNode);
    lastMark = mark;
  }
  return lastMark;
}

export function clearMessageCommentHighlights(root: HTMLElement) {
  for (const badge of Array.from(root.querySelectorAll(".agent-message-comment-badge"))) {
    badge.remove();
  }
  for (const mark of Array.from(root.querySelectorAll("mark[data-agent-message-comment-id]"))) {
    const parent = mark.parentNode;
    if (!parent) continue;
    while (mark.firstChild) parent.insertBefore(mark.firstChild, mark);
    mark.remove();
  }
  root.normalize();
}

/** Restore comment marks after a virtualized row remount or markdown rerender. */
export function restoreMessageCommentHighlights(
  root: HTMLElement,
  comments: AgentMessageComment[],
): number {
  clearMessageCommentHighlights(root);
  const content = root.textContent ?? "";
  let restored = 0;
  for (const comment of comments) {
    const range = resolveMessageTextAnchor(comment.anchor, content);
    if (!range) continue;
    const lastMark = wrapTextRange(root, range.start, range.end, comment.id);
    lastMark?.insertAdjacentElement("afterend", createCommentBadge(comment.id));
    restored++;
  }
  return restored;
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
