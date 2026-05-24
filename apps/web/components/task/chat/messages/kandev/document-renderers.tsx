"use client";

import { type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import {
  IconChecklist,
  IconFile,
  IconFileText,
  IconMessageCircle,
  IconPencil,
  IconPlus,
  IconTrash,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { markdownComponents, remarkPlugins } from "@/components/shared/markdown-components";
import {
  EmptyListNote,
  IdChip,
  KandevBody,
  KandevRow,
  KeyValueRow,
  SummaryDot,
  pluralCount,
} from "./shared";
import { pickArray, pickNumber, pickString } from "./parse";
import type { KandevRenderer } from "./types";

// MarkdownBody renders task plan / document content. We pre-trim and use the
// shared markdown component set so heading sizes, code blocks, and mermaid
// rendering match the rest of the app.
function MarkdownBody({ content }: { content: string | undefined }) {
  if (!content) return null;
  return (
    <div className="prose prose-sm dark:prose-invert max-w-none break-words">
      <ReactMarkdown remarkPlugins={remarkPlugins} components={markdownComponents}>
        {content}
      </ReactMarkdown>
    </div>
  );
}

// ContentBox is the bordered/scrollable container we use for any non-trivial
// markdown content. Capped height so a 5 000-line plan can't visually push
// the rest of the chat off-screen.
function ContentBox({ children }: { children: ReactNode }) {
  return (
    <div className="border border-border/40 rounded p-2 bg-muted/20 max-h-[400px] overflow-y-auto">
      {children}
    </div>
  );
}

function summarizeContent(content: string | undefined): string {
  if (!content) return "empty";
  const lines = content.split("\n").length;
  const chars = content.length;
  return `${lines} line${lines === 1 ? "" : "s"} · ${chars} chars`;
}

// ---------- get_task_plan ----------

export const GetTaskPlanRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const content = pickString(result, "content");
  const title = pickString(result, "title");
  const hasPlan = !!content;
  return (
    <KandevRow
      Icon={IconChecklist}
      title="Kandev: Get Task Plan"
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          <span>{hasPlan ? summarizeContent(content) : "no plan"}</span>
        </span>
      }
      status={status}
      hasExpandableContent={hasPlan}
    >
      <KandevBody>
        {title && <KeyValueRow label="title">{title}</KeyValueRow>}
        {hasPlan ? (
          <ContentBox>
            <MarkdownBody content={content} />
          </ContentBox>
        ) : (
          <EmptyListNote noun="plan" />
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- create_task_plan ----------

export const CreateTaskPlanRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const argContent = pickString(args, "content");
  const argTitle = pickString(args, "title");
  const resultId = pickString(result, "id");
  // Prefer the canonical result content when the call has finished — it
  // reflects any backend normalisation. Fall back to the arg content while
  // streaming so we don't leave the body blank.
  const displayContent = pickString(result, "content") ?? argContent;
  const displayTitle = pickString(result, "title") ?? argTitle;
  return (
    <KandevRow
      Icon={IconPlus}
      title="Kandev: Create Task Plan"
      summary={
        <span className="inline-flex items-center gap-1.5">
          <IdChip id={taskId} />
          {resultId && (
            <>
              <SummaryDot />
              <IdChip id={resultId} />
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!displayContent}
    >
      <KandevBody>
        {displayTitle && <KeyValueRow label="title">{displayTitle}</KeyValueRow>}
        {displayContent && (
          <ContentBox>
            <MarkdownBody content={displayContent} />
          </ContentBox>
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- update_task_plan ----------

export const UpdateTaskPlanRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const argContent = pickString(args, "content");
  const displayContent = pickString(result, "content") ?? argContent;
  const displayTitle = pickString(result, "title") ?? pickString(args, "title");
  return (
    <KandevRow
      Icon={IconPencil}
      title="Kandev: Update Task Plan"
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          <span>{summarizeContent(displayContent)}</span>
        </span>
      }
      status={status}
      hasExpandableContent={!!displayContent}
    >
      <KandevBody>
        {displayTitle && <KeyValueRow label="title">{displayTitle}</KeyValueRow>}
        {displayContent && (
          <ContentBox>
            <MarkdownBody content={displayContent} />
          </ContentBox>
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- delete_task_plan ----------

export const DeleteTaskPlanRenderer: KandevRenderer = ({ args, status }) => {
  const taskId = pickString(args, "task_id");
  return (
    <KandevRow
      Icon={IconTrash}
      title="Kandev: Delete Task Plan"
      summary={<IdChip id={taskId} />}
      status={status}
      hasExpandableContent={false}
    />
  );
};

// ---------- get_task_document ----------

export const GetTaskDocumentRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const docKey = pickString(args, "document_key") ?? pickString(result, "key");
  const content = pickString(result, "content");
  const title = pickString(result, "title");
  const type = pickString(result, "type");
  const author = pickString(result, "author");
  return (
    <KandevRow
      Icon={IconFileText}
      title="Kandev: Get Task Document"
      summary={
        <span className="inline-flex items-center gap-1.5">
          <IdChip id={taskId} />
          {docKey && (
            <>
              <SummaryDot />
              <span className="font-mono text-[10px]">{docKey}</span>
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!content}
    >
      <KandevBody>
        <div className="flex flex-wrap items-center gap-2">
          {title && <span className="text-sm font-medium">{title}</span>}
          {type && (
            <Badge variant="secondary" className="text-[9px]">
              {type}
            </Badge>
          )}
          {author && <span className="text-[10px] text-muted-foreground/70">by {author}</span>}
        </div>
        {content && (
          <ContentBox>
            <MarkdownBody content={content} />
          </ContentBox>
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- write_task_document ----------

export const WriteTaskDocumentRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const docKey = pickString(args, "document_key") ?? pickString(result, "key");
  const argContent = pickString(args, "content");
  const displayContent = pickString(result, "content") ?? argContent;
  const title = pickString(args, "title") ?? pickString(result, "title");
  const type = pickString(args, "type") ?? pickString(result, "type");
  return (
    <KandevRow
      Icon={IconFile}
      title="Kandev: Write Task Document"
      summary={
        <span className="inline-flex items-center gap-1.5">
          <IdChip id={taskId} />
          {docKey && (
            <>
              <SummaryDot />
              <span className="font-mono text-[10px]">{docKey}</span>
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={!!displayContent}
    >
      <KandevBody>
        <div className="flex flex-wrap items-center gap-2">
          {title && <span className="text-sm font-medium">{title}</span>}
          {type && (
            <Badge variant="secondary" className="text-[9px]">
              {type}
            </Badge>
          )}
        </div>
        {displayContent && (
          <ContentBox>
            <MarkdownBody content={displayContent} />
          </ContentBox>
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- get_task_conversation ----------

type ConversationMessage = {
  id?: string;
  author_type?: string;
  type?: string;
  content?: string;
  created_at?: string;
};

const MAX_INLINE_MESSAGES = 30;

function ConversationMessageRow({ msg }: { msg: ConversationMessage }) {
  const isUser = msg.author_type === "user";
  // Render the author label as a small uppercase tag rather than a coloured
  // bubble — the chat is already inside a tool-call card, so a heavy bubble
  // style would visually drown out the surrounding messages.
  return (
    <div className="text-xs space-y-0.5">
      <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-wide text-muted-foreground/70">
        <span>{isUser ? "user" : (msg.author_type ?? "agent")}</span>
        {msg.type && msg.type !== "message" && (
          <Badge variant="outline" className="text-[9px]">
            {msg.type}
          </Badge>
        )}
      </div>
      {msg.content && (
        <div className="whitespace-pre-wrap break-words text-foreground">{msg.content}</div>
      )}
    </div>
  );
}

export const GetTaskConversationRenderer: KandevRenderer = ({ args, result, status }) => {
  const taskId = pickString(args, "task_id");
  const sessionId = pickString(args, "session_id") ?? pickString(result, "session_id");
  const messages = pickArray<ConversationMessage>(result, "messages") ?? [];
  // The backend paginates: `total` is the absolute count, `messages.length`
  // is just the current page. The "more not shown" footer must account for
  // both the inline-cap *and* any server-side pagination, otherwise a
  // capped page (total=200, messages=50) reads as if everything was visible.
  const total = pickNumber(result, "total") ?? messages.length;
  const visible = messages.slice(0, MAX_INLINE_MESSAGES);
  const hiddenCount = Math.max(0, total - visible.length);
  const truncated = hiddenCount > 0;
  return (
    <KandevRow
      Icon={IconMessageCircle}
      title="Kandev: Get Task Conversation"
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && <IdChip id={taskId} />}
          {sessionId && (
            <>
              {taskId && <SummaryDot />}
              <IdChip id={sessionId} />
            </>
          )}
          {(taskId || sessionId) && <SummaryDot />}
          {pluralCount(total, "message")}
        </span>
      }
      status={status}
      hasExpandableContent={messages.length > 0}
    >
      <KandevBody>
        {messages.length === 0 ? (
          <EmptyListNote noun="messages" />
        ) : (
          <div className="space-y-2 max-h-[400px] overflow-y-auto">
            {visible.map((m, i) => (
              <ConversationMessageRow key={m.id ?? i} msg={m} />
            ))}
            {truncated && (
              <div className="text-[10px] italic text-muted-foreground/70">
                + {hiddenCount} more not shown
              </div>
            )}
          </div>
        )}
      </KandevBody>
    </KandevRow>
  );
};
