import { IconMessage } from "@tabler/icons-react";
import type { MessageCommentDecoration } from "@/lib/chat/agent-message-comments";

function MessageCommentBadges({
  decorations,
  onOpen,
}: {
  decorations: MessageCommentDecoration[];
  onOpen: (commentId: string, position: { x: number; y: number }) => void;
}) {
  return decorations.map((decoration) => (
    <button
      key={decoration.comment.id}
      type="button"
      className="comment-badge agent-message-comment-badge border-0 bg-transparent p-0"
      style={{ position: "absolute", left: decoration.left, top: decoration.top }}
      data-agent-message-comment-id={decoration.comment.id}
      data-comment-id={decoration.comment.id}
      aria-label="Edit comment"
      onClick={(event) => {
        event.preventDefault();
        event.stopPropagation();
        const rect = event.currentTarget.getBoundingClientRect();
        onOpen(decoration.comment.id, { x: rect.right, y: rect.bottom });
      }}
    >
      <IconMessage aria-hidden="true" />
    </button>
  ));
}

function MessageCommentFallbackHighlights({
  decorations,
}: {
  decorations: MessageCommentDecoration[];
}) {
  return decorations.flatMap((decoration) =>
    decoration.highlightRects.map((rect, index) => (
      <span
        key={`${decoration.comment.id}:${index}`}
        aria-hidden="true"
        className="agent-message-comment-fallback"
        data-agent-message-comment-fallback="true"
        data-comment-id={decoration.comment.id}
        style={rect}
      />
    )),
  );
}

export function MessageCommentDecorationOverlay({
  decorations,
  useCustomHighlights,
  onOpen,
}: {
  decorations: MessageCommentDecoration[];
  useCustomHighlights: boolean;
  onOpen: (commentId: string, position: { x: number; y: number }) => void;
}) {
  return (
    <>
      {!useCustomHighlights ? <MessageCommentFallbackHighlights decorations={decorations} /> : null}
      <MessageCommentBadges decorations={decorations} onOpen={onOpen} />
    </>
  );
}

export function MessageCustomHighlightStyle({ highlightName }: { highlightName: string }) {
  return (
    <style>{`::highlight(${highlightName}) {
      background-color: color-mix(in oklch, var(--accent) 70%, transparent);
      color: inherit;
      text-decoration: underline 2px color-mix(in oklch, var(--accent-foreground) 25%, transparent);
      text-underline-offset: 2px;
    }`}</style>
  );
}
