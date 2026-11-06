"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { ClarificationQuestion } from "@/lib/types/http";
import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";
import { QuestionImages } from "@/components/task/chat/question-images";

export function AsyncQuestionForm({
  question,
  selections,
  customText,
  submitting,
  error,
  onSelect,
  onCustomText,
  onReject,
  onSubmit,
  includeActions = true,
}: {
  question: ClarificationInboxAsyncQuestion;
  selections: Record<string, string>;
  customText: Record<string, string>;
  submitting: boolean;
  error: string | null;
  onSelect: (questionId: string, optionId: string) => void;
  onCustomText: (questionId: string, value: string) => void;
  onReject: () => void;
  onSubmit: () => void;
  includeActions?: boolean;
}) {
  return (
    <div className="mt-3 space-y-4">
      {question.context && (
        <p className="whitespace-pre-wrap text-sm text-muted-foreground">{question.context}</p>
      )}
      {question.questions.map((item, index) => (
        <AsyncQuestionItem
          key={item.id}
          item={item}
          index={index}
          total={question.questions.length}
          pendingId={question.question_id}
          sessionId={question.session_id}
          selectedOption={selections[item.id]}
          customValue={customText[item.id] ?? ""}
          submitting={submitting}
          onSelect={onSelect}
          onCustomText={onCustomText}
        />
      ))}
      {error && (
        <p className="text-sm text-destructive" role="alert" data-testid="async-question-error">
          {error}
        </p>
      )}
      {includeActions && (
        <AsyncQuestionActions submitting={submitting} onReject={onReject} onSubmit={onSubmit} />
      )}
    </div>
  );
}

export function AsyncQuestionActions({
  submitting,
  onReject,
  onSubmit,
}: {
  submitting: boolean;
  onReject: () => void;
  onSubmit: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-end gap-2">
      <Button
        type="button"
        variant="ghost"
        className="min-h-7 cursor-pointer px-4 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={onReject}
        disabled={submitting}
        data-testid="async-question-reject"
      >
        {t("needsYouInbox:asyncReject")}
      </Button>
      <Button
        type="button"
        className="min-h-7 cursor-pointer px-4 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        onClick={onSubmit}
        disabled={submitting}
        data-testid="async-question-submit"
      >
        {submitting ? t("needsYouInbox:asyncSubmitting") : t("needsYouInbox:asyncSubmit")}
      </Button>
    </div>
  );
}

function AsyncQuestionItem({
  item,
  index,
  total,
  pendingId,
  sessionId,
  selectedOption,
  customValue,
  submitting,
  onSelect,
  onCustomText,
}: {
  item: ClarificationQuestion;
  index: number;
  total: number;
  pendingId: string;
  sessionId: string;
  selectedOption: string | undefined;
  customValue: string;
  submitting: boolean;
  onSelect: (questionId: string, optionId: string) => void;
  onCustomText: (questionId: string, value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <fieldset className="space-y-2" data-testid="async-question-item">
      <QuestionImages
        pendingId={pendingId}
        questionId={item.id}
        sessionId={sessionId}
        images={item.images}
        apiPath="agent-questions"
      />
      <legend className="text-sm font-medium">
        {total > 1 ? (
          <span className="mr-1 text-xs text-muted-foreground">
            {t("needsYouInbox:asyncQuestionOf", { current: index + 1, total })}
          </span>
        ) : null}
        {item.prompt}
      </legend>
      <div className="space-y-1.5">
        {item.options.map((option) => (
          <AsyncQuestionOption
            key={option.option_id}
            option={option}
            selected={selectedOption === option.option_id}
            submitting={submitting}
            onSelect={() => onSelect(item.id, option.option_id)}
          />
        ))}
      </div>
      {item.options
        .filter((option) => option.images && option.images.length > 0)
        .map((option) => (
          <div key={`images-${option.option_id}`} className="pl-3">
            <p className="text-xs text-muted-foreground">{option.label}</p>
            <QuestionImages
              pendingId={pendingId}
              questionId={item.id}
              sessionId={sessionId}
              images={option.images}
              apiPath="agent-questions"
            />
          </div>
        ))}
      {item.allow_custom_text !== false && (
        <label className="block">
          <span className="mb-1 block text-xs text-muted-foreground">
            {t("needsYouInbox:asyncAdditionalAnswer")}
          </span>
          <textarea
            value={customValue}
            onChange={(event) => onCustomText(item.id, event.target.value)}
            disabled={submitting}
            rows={2}
            className="w-full resize-y rounded-md border border-border bg-background px-3 py-2 text-sm outline-none focus:border-blue-500/60"
            data-testid="async-question-custom-text"
          />
        </label>
      )}
    </fieldset>
  );
}

function AsyncQuestionOption({
  option,
  selected,
  submitting,
  onSelect,
}: {
  option: ClarificationQuestion["options"][number];
  selected: boolean;
  submitting: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={submitting}
      aria-pressed={selected}
      data-testid="async-question-option"
      data-selected={selected ? "true" : "false"}
      className={cn(
        "flex min-h-7 w-full items-start gap-3 rounded-lg border px-3 py-2 text-left text-sm transition-colors max-md:min-h-11 [@media(pointer:coarse)]:min-h-11",
        selected
          ? "border-blue-500/50 bg-blue-500/15 text-foreground"
          : "border-border text-foreground/90 hover:border-border/80 hover:bg-muted/40",
        submitting ? "cursor-not-allowed opacity-60" : "cursor-pointer",
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "mt-1 h-3.5 w-3.5 shrink-0 rounded-full border",
          selected ? "border-blue-500 bg-blue-500" : "border-muted-foreground/50",
        )}
      />
      <span className="min-w-0">
        <span className="block font-medium">{option.label}</span>
        {option.description && (
          <span className="block text-xs text-muted-foreground">{option.description}</span>
        )}
      </span>
    </button>
  );
}
