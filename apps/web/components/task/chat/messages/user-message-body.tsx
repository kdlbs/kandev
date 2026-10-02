"use client";

import { useMemo, useState } from "react";
import type { Components } from "react-markdown";
import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { BoundedMessagePreview } from "./bounded-message-preview";
import { cn } from "@/lib/utils";
import { t } from "@/lib/i18n";
import {
  getMessagePreview,
  MESSAGE_PREVIEW_MAX_CODE_UNITS,
  MESSAGE_PREVIEW_MAX_LINES,
} from "@/lib/utils/message-preview";
import { splitMessageSegments } from "@/lib/utils/workflow-instructions";

type UserMessageBodyOptions = {
  hasContent: boolean;
  showRaw: boolean;
  hasAttachments: boolean;
  content: string;
  rawContent?: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  /** Message's originating task, from `MessageTaskOriginContext`. Only the
   *  string `"coordinator"` triggers the About-prefix tag below. */
  taskOrigin?: string;
};

// i18n-exempt: stable, coordinator-agent-facing wire marker (English, not i18n), not rendered as-is.
const COORDINATOR_ABOUT_PREFIX = "About ";
const COORDINATOR_ABOUT_SEPARATOR = ": ";
// i18n-exempt: stable, coordinator-agent-facing wire marker (English, not i18n), not rendered as-is.
const COORDINATOR_REFERENCED_PREFIX_RE =
  /^About (.+?) \[(task|proposal|stall|workflow):([^\]\s]+)\]: /;

type CoordinatorAboutPrefix = { id: string; remainder: string };

/** Parses the referenced form, "About <id> [<kind>:<ref>]: ", added for
 *  `get_coordinator_item_kandev` reads (docs/specs/coordinator/system-design/
 *  copilot-panel.md#ask-about-this). `<id>` is the shortest run of non-newline
 *  characters followed by a bracketed `task`/`proposal`/`stall`/`workflow` reference and
 *  `: `, so an id containing its own `: `, `[` or `]` is still read back
 *  whole, and a second bracketed-looking sequence later in the message is
 *  never mistaken for the prefix's own. Returns `null` for content that does
 *  not match, or whose matched remainder is empty. */
function parseCoordinatorReferencedPrefix(content: string): CoordinatorAboutPrefix | null {
  const match = COORDINATOR_REFERENCED_PREFIX_RE.exec(content);
  if (!match) return null;
  const remainder = content.slice(match[0].length);
  if (!remainder) return null;
  return { id: match[1], remainder };
}

/** Parses the legacy "About <id>: " prefix of earlier messages, predating the
 *  bracketed reference. The id is the text between `About ` and the first
 *  `: `; an id containing its own `: ` splits at that first occurrence (a
 *  known, accepted limit). Returns `null` for content that does not start
 *  with the prefix, has no `: ` separator, whose id is empty or spans a line
 *  break, or whose matched remainder is empty. */
function parseCoordinatorLegacyAboutPrefix(content: string): CoordinatorAboutPrefix | null {
  if (!content.startsWith(COORDINATOR_ABOUT_PREFIX)) return null;
  const separatorIndex = content.indexOf(
    COORDINATOR_ABOUT_SEPARATOR,
    COORDINATOR_ABOUT_PREFIX.length,
  );
  if (separatorIndex === -1) return null;
  const id = content.slice(COORDINATOR_ABOUT_PREFIX.length, separatorIndex);
  if (!id || /[\r\n]/.test(id)) return null;
  const remainder = content.slice(separatorIndex + COORDINATOR_ABOUT_SEPARATOR.length);
  if (!remainder) return null;
  return { id, remainder };
}

/** Tries the referenced form first, then the legacy form of earlier messages
 *  (docs/specs/coordinator/system-design/copilot-panel.md#ask-about-this). A
 *  text matching neither, or whose matched remainder is empty, is not a
 *  match: the caller renders it unchanged with no tag. */
function parseCoordinatorAboutPrefix(content: string): CoordinatorAboutPrefix | null {
  return parseCoordinatorReferencedPrefix(content) ?? parseCoordinatorLegacyAboutPrefix(content);
}

function CoordinatorAboutTag({ id }: { id: string }) {
  return (
    <Badge data-testid="coordinator-about-tag" variant="secondary" className="w-fit">
      {t("chat:coordinatorAboutTag", { id })}
    </Badge>
  );
}

function UserMessageMarkdown({
  content,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  content: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  return (
    <div className="markdown-body markdown-body-user max-w-none">
      <MemoizedMarkdown
        content={content}
        taskId={taskId}
        components={promptMentionComponents}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
      />
    </div>
  );
}

function CollapsedInstructions({
  label,
  testId,
  instructions,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  label: string;
  testId: string;
  instructions: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger
        className="flex w-full cursor-pointer items-center gap-1 rounded-md bg-muted/40 px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted/60"
        data-testid={testId}
      >
        <IconChevronRight
          className={cn("h-3.5 w-3.5 shrink-0 transition-transform", open && "rotate-90")}
        />
        <span>{label}</span>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-2">
        <BoundedMessagePreview
          source={instructions}
          fileName="kandev-workflow-instructions.txt"
          renderContent={(preview) => (
            <UserMessageMarkdown
              content={preview}
              promptMentionComponents={promptMentionComponents}
              taskId={taskId}
              worktreePath={worktreePath}
              onOpenFile={onOpenFile}
            />
          )}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function MessageSegments({
  content,
  downloadSource = content,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  content: string;
  downloadSource?: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  const { t } = useTranslation();
  const segments = useMemo(() => {
    let remainingLines = MESSAGE_PREVIEW_MAX_LINES;
    let remainingCodeUnits = MESSAGE_PREVIEW_MAX_CODE_UNITS;
    return splitMessageSegments(content).map((segment) => {
      if (segment.type === "instructions") {
        return { segment, preview: getMessagePreview(segment.content) };
      }
      const preview = getMessagePreview(segment.content, {
        maxLines: remainingLines,
        maxCodeUnits: remainingCodeUnits,
      });
      remainingLines = Math.max(0, remainingLines - preview.logicalLines);
      remainingCodeUnits = Math.max(0, remainingCodeUnits - preview.codeUnits);
      return { segment, preview };
    });
  }, [content]);
  return (
    <div className="space-y-2">
      {segments.map(({ segment, preview }, index) => {
        if (segment.type === "text") {
          return (
            <BoundedMessagePreview
              key={`text-${index}`}
              source={segment.content}
              downloadSource={downloadSource}
              fileName="kandev-message.txt"
              preview={preview}
              renderContent={(previewContent) => (
                <UserMessageMarkdown
                  content={previewContent}
                  promptMentionComponents={promptMentionComponents}
                  taskId={taskId}
                  worktreePath={worktreePath}
                  onOpenFile={onOpenFile}
                />
              )}
            />
          );
        }
        const isMove = segment.kind === "move";
        return (
          <CollapsedInstructions
            key={`instructions-${segment.kind}-${index}`}
            label={t(
              isMove
                ? "workflows:workflowMoveInstructionsCollapsed"
                : "workflows:workflowInstructionsCollapsed",
            )}
            testId={isMove ? "workflow-move-instructions-toggle" : "workflow-instructions-toggle"}
            instructions={segment.content}
            promptMentionComponents={promptMentionComponents}
            taskId={taskId}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
          />
        );
      })}
    </div>
  );
}

function CoordinatorAboutMessage({
  id,
  remainder,
  content,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  id: string;
  remainder: string;
  content: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  return (
    <div className="space-y-2">
      <MessageSegments
        content={remainder}
        downloadSource={content}
        promptMentionComponents={promptMentionComponents}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
      />
      <CoordinatorAboutTag id={id} />
    </div>
  );
}

export function renderUserMessageBody({
  hasContent,
  showRaw,
  hasAttachments,
  content,
  rawContent,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
  taskOrigin,
}: UserMessageBodyOptions): React.ReactNode {
  if (hasContent && showRaw) {
    const raw = rawContent || content;
    return (
      <BoundedMessagePreview
        source={raw}
        fileName="kandev-message.txt"
        renderContent={(preview) => (
          <pre className="whitespace-pre-wrap font-mono text-xs">{preview}</pre>
        )}
      />
    );
  }
  if (hasContent) {
    const coordinatorPrefix =
      taskOrigin === "coordinator" ? parseCoordinatorAboutPrefix(content) : null;
    if (coordinatorPrefix) {
      return (
        <CoordinatorAboutMessage
          id={coordinatorPrefix.id}
          remainder={coordinatorPrefix.remainder}
          content={content}
          promptMentionComponents={promptMentionComponents}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
        />
      );
    }
    return (
      <MessageSegments
        content={content}
        promptMentionComponents={promptMentionComponents}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
      />
    );
  }
  if (!hasAttachments) {
    return (
      <p className="whitespace-pre-wrap break-words overflow-wrap-anywhere">{t("task:empty")}</p>
    );
  }
  return null;
}
