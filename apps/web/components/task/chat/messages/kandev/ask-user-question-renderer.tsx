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
import { pickArray, pickObject, pickString } from "./parse";
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

function matchAnswerForQuestion(
  responses: Record<string, unknown> | undefined,
  question: Question,
  index: number,
): AnswerEntry | undefined {
  if (!responses) return undefined;
  // Responses may come back keyed by question id OR as a flat list — handle
  // both shapes defensively so a backend tweak doesn't blank the UI.
  if (question.id && responses[question.id]) {
    return responses[question.id] as AnswerEntry;
  }
  const arr = Array.isArray(responses) ? responses : undefined;
  return arr?.[index];
}

export const AskUserQuestionRenderer: KandevRenderer = ({ args, result, status }) => {
  const questions = pickArray<Question>(args, "questions") ?? [];
  const context = pickString(args, "context");
  const responses = pickObject(result, "responses");
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
            Awaiting user response (pending_id={pendingId.slice(0, 8)}…)
          </div>
        )}
      </KandevBody>
    </KandevRow>
  );
};
