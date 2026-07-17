"use client";

import { Badge } from "@kandev/ui/badge";
import { ScrollArea } from "@kandev/ui/scroll-area";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";

import type { SnapshotPreview, SnapshotPreviewMessage } from "@/lib/api/domains/share-api";

type Props = {
  snapshot: SnapshotPreview;
};

// Long conversations would blow up the dialog. Cap what we render in the
// preview to the first + last few messages; the published gist still has
// the complete snapshot.
const PREVIEW_HEAD = 8;
const PREVIEW_TAIL = 8;
const previewRemarkPlugins = [remarkGfm];

const previewMarkdownComponents: Components = {
  table: ({ children }) => (
    <div className="overflow-x-auto">
      <table>{children}</table>
    </div>
  ),
};

/**
 * Renders the redacted snapshot as a read-only preview. Intentionally not
 * reusing NativeMessageList: the snapshot does not have the live Zustand
 * shape the rich renderer expects, and a stripped-down view makes "this is
 * exactly what will be published" easy to verify at a glance.
 */
export function ShareSnapshotPreview({ snapshot }: Props) {
  const slices = previewSlices(snapshot.messages);
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        {snapshot.session.agent_type && (
          <Badge variant="outline">{snapshot.session.agent_type}</Badge>
        )}
        {snapshot.session.model && <Badge variant="outline">{snapshot.session.model}</Badge>}
        {snapshot.session.executor_type && (
          <Badge variant="outline">{snapshot.session.executor_type}</Badge>
        )}
        <span>{snapshot.messages.length} messages</span>
        {snapshot.redaction.applied_rules.length > 0 && (
          <span>Redacted: {snapshot.redaction.applied_rules.join(", ")}</span>
        )}
      </div>
      <ScrollArea className="h-72 rounded border bg-muted/30">
        <div className="flex flex-col gap-3 p-3">
          {snapshot.messages.length === 0 && (
            <p className="text-sm text-muted-foreground italic">
              This conversation has no shareable content.
            </p>
          )}
          {slices.head.map((msg, idx) => (
            <PreviewMessage key={`h-${idx}`} message={msg} />
          ))}
          {slices.hiddenCount > 0 && (
            <p className="text-center text-xs text-muted-foreground italic py-1">
              … {slices.hiddenCount} more message{slices.hiddenCount === 1 ? "" : "s"} hidden in
              this preview (still included in the share) …
            </p>
          )}
          {slices.tail.map((msg, idx) => (
            <PreviewMessage key={`t-${idx}`} message={msg} />
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}

// previewSlices splits the messages into a head + tail with a hidden count
// in between when the conversation exceeds the per-section caps. Short
// conversations come back as the full head with empty tail / zero hidden.
function previewSlices(messages: SnapshotPreviewMessage[]): {
  head: SnapshotPreviewMessage[];
  tail: SnapshotPreviewMessage[];
  hiddenCount: number;
} {
  if (messages.length <= PREVIEW_HEAD + PREVIEW_TAIL) {
    return { head: messages, tail: [], hiddenCount: 0 };
  }
  return {
    head: messages.slice(0, PREVIEW_HEAD),
    tail: messages.slice(messages.length - PREVIEW_TAIL),
    hiddenCount: messages.length - PREVIEW_HEAD - PREVIEW_TAIL,
  };
}

function PreviewMessage({ message }: { message: SnapshotPreviewMessage }) {
  return (
    <div className="flex min-w-0 flex-col gap-1 rounded border border-border/60 bg-background p-2">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">
        {message.role}
      </div>
      <div className="flex min-w-0 flex-col gap-1">
        {message.blocks.map((block, i) => (
          <PreviewBlock key={i} block={block} />
        ))}
      </div>
    </div>
  );
}

function PreviewBlock({ block }: { block: SnapshotPreviewMessage["blocks"][number] }) {
  if (block.kind === "text") {
    return (
      <div className="markdown-body min-w-0 text-sm">
        <ReactMarkdown remarkPlugins={previewRemarkPlugins} components={previewMarkdownComponents}>
          {block.text ?? ""}
        </ReactMarkdown>
      </div>
    );
  }
  if (block.kind === "tool_call") {
    return (
      <div className="min-w-0 rounded bg-muted/60 p-1.5 text-xs">
        <div className="truncate font-mono" title={block.tool_name || "tool"}>
          {block.tool_name || "tool"}
        </div>
        {block.text && (
          <p className="truncate text-muted-foreground" title={block.text}>
            {block.text}
          </p>
        )}
      </div>
    );
  }
  if (block.kind === "tool_result") {
    return (
      <pre className="max-w-full overflow-x-auto rounded bg-muted/60 p-1.5 text-xs">
        <code>{block.output}</code>
      </pre>
    );
  }
  if (block.kind === "diff") {
    return (
      <div className="min-w-0 rounded bg-muted/60 p-1.5 text-xs">
        <div className="truncate font-mono" title={block.path}>
          {block.path}
        </div>
        <pre className="max-w-full overflow-x-auto whitespace-pre">
          <code>{block.unified_diff}</code>
        </pre>
      </div>
    );
  }
  return null;
}
