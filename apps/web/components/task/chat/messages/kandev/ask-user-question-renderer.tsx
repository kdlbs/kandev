"use client";

import { IconHelpHexagon } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { EmptyListNote, KandevBody, KandevRow, KeyValueRow, SummaryDot } from "./shared";
import { pickArray, pickString, shortId } from "./parse";
import type { KandevRenderer } from "./types";
import { useTranslation } from "react-i18next";
import { ClarificationMarkdown } from "../../clarification-markdown";

type QuestionOption = { label?: string; description?: string };
type Question = {
  id?: string;
  title?: string;
  prompt?: string;
  options?: QuestionOption[];
};

type AnswerEntry = { question_id?: string; selected?: string; custom_text?: string };

// QuestionBlock renders a single question with its options and (if available)
// the user's answer underlined in the body so a completed call is informative
// at a glance.
function QuestionBlock({ q, answer }: { q: Question; answer: AnswerEntry | undefined }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      {q.title && (
        <ClarificationMarkdown
          variant="inline"
          className="text-[11px] font-medium text-muted-foreground"
        >
          {q.title}
        </ClarificationMarkdown>
      )}
      {q.prompt && (
        <ClarificationMarkdown variant="block" className="text-xs text-foreground">
          {q.prompt}
        </ClarificationMarkdown>
      )}
      {q.options && q.options.length > 0 && (
        <div className="space-y-1">
          {q.options.map((opt, i) => {
            const isSelected = answer?.selected === opt.label;
            return (
              <div key={`${opt.label ?? i}`} className="flex min-w-0 items-start gap-2">
                <Badge
                  variant={isSelected ? "default" : "outline"}
                  className="h-auto min-h-5 max-w-full shrink whitespace-normal overflow-visible text-left text-[10px] leading-tight"
                >
                  <ClarificationMarkdown variant="inline">
                    {opt.label ?? t("task:option", { index: i + 1 })}
                  </ClarificationMarkdown>
                </Badge>
                {opt.description && (
                  <ClarificationMarkdown
                    variant="inline"
                    className="min-w-0 flex-1 text-[10px] leading-relaxed text-muted-foreground"
                  >
                    {opt.description}
                  </ClarificationMarkdown>
                )}
              </div>
            );
          })}
        </div>
      )}
      {answer?.custom_text && (
        <KeyValueRow label={t("task:answer")}>
          <span className="whitespace-pre-wrap">{answer.custom_text}</span>
        </KeyValueRow>
      )}
    </div>
  );
}

// matchAnswerForQuestion accepts both response shapes the backend may emit:
// keyed by question id (`{ q1: {...} }`) or as a positional list. The raw
// `responses` value is read directly rather than via `pickObject`, because
// the latter discards arrays and would silently lose list-shaped payloads.
function matchAnswerForQuestion(
  responses: unknown,
  question: Question,
  index: number,
): AnswerEntry | undefined {
  if (!responses || typeof responses !== "object") return undefined;
  if (Array.isArray(responses)) return responses[index] as AnswerEntry | undefined;
  if (question.id) {
    const entry = (responses as Record<string, unknown>)[question.id];
    if (entry) return entry as AnswerEntry;
  }
  return undefined;
}

function readResponses(result: unknown): unknown {
  if (!result || typeof result !== "object") return undefined;
  return (result as Record<string, unknown>).responses;
}

export const AskUserQuestionRenderer: KandevRenderer = ({ args, result, status }) => {
  const { t } = useTranslation();
  const questions = pickArray<Question>(args, "questions") ?? [];
  const context = pickString(args, "context");
  const responses = readResponses(result);
  const pendingId = pickString(result, "pending_id");

  // Build a short header summary: count of questions, plus the first prompt
  // truncated so the row stays single-line.
  const firstPrompt = questions[0]?.prompt;
  const promptShort = firstPrompt ? firstPrompt.replace(/\s+/g, " ").trim() : undefined;

  return (
    <div data-testid="ask-user-question-renderer">
      <KandevRow
        Icon={IconHelpHexagon}
        title={t("task:kandevAskUserQuestion")}
        summary={
          <span className="inline-flex items-center gap-1.5 min-w-0">
            <span>{t("task:questionCount", { count: questions.length })}</span>
            {promptShort && (
              <>
                <SummaryDot />
                <span className="truncate max-w-[50ch]">
                  &ldquo;
                  <ClarificationMarkdown variant="inline" linkBehavior="passive" className="inline">
                    {promptShort}
                  </ClarificationMarkdown>
                  &rdquo;
                </span>
              </>
            )}
          </span>
        }
        status={status}
        hasExpandableContent={questions.length > 0 || !!context}
      >
        <KandevBody>
          {context && (
            <KeyValueRow label={t("task:contextFieldLabel")}>
              <span className="whitespace-pre-wrap">{context}</span>
            </KeyValueRow>
          )}
          {questions.length === 0 ? (
            <EmptyListNote messageKey="task:noQuestionsFound" />
          ) : (
            <div className="space-y-3">
              {questions.map((q, i) => (
                <QuestionBlock
                  key={q.id ?? i}
                  q={q}
                  answer={matchAnswerForQuestion(responses, q, i)}
                />
              ))}
            </div>
          )}
          {pendingId && status === "running" && (
            <div className="text-[10px] italic text-muted-foreground/70">
              {t("task:awaitingUserResponse", { pendingId: shortId(pendingId) })}
            </div>
          )}
        </KandevBody>
      </KandevRow>
    </div>
  );
};
