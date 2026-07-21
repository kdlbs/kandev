"use client";

import { IconHelpHexagon } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import {
  EmptyListNote,
  KandevBody,
  KandevRow,
  KeyValueRow,
  SummaryDot,
  pluralCount,
} from "./shared";
import { pickArray, pickString, shortId } from "./parse";
import type { KandevRenderer } from "./types";

type QuestionOption = { label?: string; description?: string };
type Question = {
  id?: string;
  prompt?: string;
  options?: QuestionOption[];
};

type AnswerEntry = { question_id?: string; selected?: string; custom_text?: string };

// QuestionBlock renders a single question with its options and (if available)
// the user's answer underlined in the body so a completed call is informative
// at a glance.
function QuestionBlock({ q, answer }: { q: Question; answer: AnswerEntry | undefined }) {
  return (
    <div className="space-y-1.5">
      {q.prompt && <div className="text-xs text-foreground whitespace-pre-wrap">{q.prompt}</div>}
      {q.options && q.options.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {q.options.map((opt, i) => {
            const isSelected = answer?.selected === opt.label;
            return (
              <Badge
                key={`${opt.label ?? i}`}
                variant={isSelected ? "default" : "outline"}
                className="text-[10px]"
                title={opt.description}
              >
                {opt.label ?? `option ${i + 1}`}
              </Badge>
            );
          })}
        </div>
      )}
      {answer?.custom_text && (
        <KeyValueRow label="answer">
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
  const questions = pickArray<Question>(args, "questions") ?? [];
  const context = pickString(args, "context");
  const responses = readResponses(result);
  const pendingId = pickString(result, "pending_id");

  // Build a short header summary: count of questions, plus the first prompt
  // truncated so the row stays single-line.
  const firstPrompt = questions[0]?.prompt;
  const promptShort = firstPrompt ? firstPrompt.replace(/\s+/g, " ").trim() : undefined;

  return (
    <KandevRow
      Icon={IconHelpHexagon}
      title="Kandev: Ask User Question"
      summary={
        <span className="inline-flex items-center gap-1.5 min-w-0">
          <span>{pluralCount(questions.length, "question")}</span>
          {promptShort && (
            <>
              <SummaryDot />
              <span className="truncate max-w-[50ch]">&ldquo;{promptShort}&rdquo;</span>
            </>
          )}
        </span>
      }
      status={status}
      hasExpandableContent={questions.length > 0 || !!context}
    >
      <KandevBody>
        {context && (
          <KeyValueRow label="context">
            <span className="whitespace-pre-wrap">{context}</span>
          </KeyValueRow>
        )}
        {questions.length === 0 ? (
          <EmptyListNote noun="questions" />
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
            Awaiting user response (pending_id={shortId(pendingId)})
          </div>
        )}
      </KandevBody>
    </KandevRow>
  );
};
