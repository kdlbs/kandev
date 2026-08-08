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
import { EmptyListNote, IdChip, KandevBody, KandevRow, KeyValueRow, SummaryDot } from "./shared";
import { pickArray, pickNumber, pickString } from "./parse";
import type { KandevRenderer } from "./types";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

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

// Two counts in one label, but i18next selects a plural form on `count` alone.
// Lines own `count`; the character total rides along as a plain value.
function summarizeContent(t: TFunction, content: string | undefined): string {
  if (!content) return t("task:documentSummaryEmpty");
  const lines = content.split("\n").length;
  const chars = content.length;
  return t("task:documentSummary", { count: lines, chars });
}

// ---------- get_task_plan ----------

export const GetTaskPlanRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const taskId = pickString(args, "task_id");
  const content = pickString(result, "content");
  const title = pickString(result, "title");
  const hasPlan = !!content;
  return (
    <KandevRow
      Icon={IconChecklist}
      title={t("task:kandevGetTaskPlan")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          <span>{hasPlan ? summarizeContent(t, content) : t("task:noPlan")}</span>
        </span>
      }
      status={status}
      hasExpandableContent={hasPlan}
    >
      <KandevBody>
        {title && <KeyValueRow label={t("task:title")}>{title}</KeyValueRow>}
        {hasPlan ? (
          <ContentBox>
            <MarkdownBody content={content} />
          </ContentBox>
        ) : (
          <EmptyListNote messageKey="task:noPlanFound" />
        )}
      </KandevBody>
    </KandevRow>
  );
};

// ---------- create_task_plan ----------

export const CreateTaskPlanRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
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
      title={t("task:kandevCreateTaskPlan")}
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
        {displayTitle && <KeyValueRow label={t("task:title")}>{displayTitle}</KeyValueRow>}
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
  const { t } = useTranslation();
  const taskId = pickString(args, "task_id");
  const argContent = pickString(args, "content");
  const displayContent = pickString(result, "content") ?? argContent;
  const displayTitle = pickString(result, "title") ?? pickString(args, "title");
  return (
    <KandevRow
      Icon={IconPencil}
      title={t("task:kandevUpdateTaskPlan")}
      summary={
        <span className="inline-flex items-center gap-1.5">
          {taskId && (
            <>
              <IdChip id={taskId} />
              <SummaryDot />
            </>
          )}
          <span>{summarizeContent(t, displayContent)}</span>
        </span>
      }
      status={status}
      hasExpandableContent={!!displayContent}
    >
      <KandevBody>
        {displayTitle && <KeyValueRow label={t("task:title")}>{displayTitle}</KeyValueRow>}
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
  const { t } = useTranslation();
  const taskId = pickString(args, "task_id");
  return (
    <KandevRow
      Icon={IconTrash}
      title={t("task:kandevDeleteTaskPlan")}
      summary={<IdChip id={taskId} />}
      status={status}
      hasExpandableContent={false}
    />
  );
};

// ---------- get_task_document ----------

export const GetTaskDocumentRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const taskId = pickString(args, "task_id");
  const docKey = pickString(args, "document_key") ?? pickString(result, "key");
  const content = pickString(result, "content");
  const title = pickString(result, "title");
  const type = pickString(result, "type");
  const author = pickString(result, "author");
  return (
    <KandevRow
      Icon={IconFileText}
      title={t("task:kandevGetTaskDocument")}
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
  const { t } = useTranslation();
  const taskId = pickString(args, "task_id");
  const docKey = pickString(args, "document_key") ?? pickString(result, "key");
  const argContent = pickString(args, "content");
  const displayContent = pickString(result, "content") ?? argContent;
  const title = pickString(args, "title") ?? pickString(result, "title");
  const type = pickString(args, "type") ?? pickString(result, "type");
  return (
    <KandevRow
      Icon={IconFile}
      title={t("task:kandevWriteTaskDocument")}
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
  const { t } = useTranslation();
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
      title={t("task:kandevGetTaskConversation")}
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
          {t("task:messageCount", { count: total })}
        </span>
      }
      status={status}
      hasExpandableContent={messages.length > 0}
    >
      <KandevBody>
        {messages.length === 0 ? (
          <EmptyListNote messageKey="task:noMessagesFound" />
        ) : (
          <div className="space-y-2 max-h-[400px] overflow-y-auto">
            {visible.map((m, i) => (
              <ConversationMessageRow key={m.id ?? i} msg={m} />
            ))}
            {truncated && (
              <div className="text-[10px] italic text-muted-foreground/70">
                {t("task:moreNotShown", { count: hiddenCount })}
              </div>
            )}
          </div>
        )}
      </KandevBody>
    </KandevRow>
  );
};
