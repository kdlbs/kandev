"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import type { PromptConflictReview } from "@/hooks/domains/settings/use-prompt-conflict-review";
import type { PromptConflictChoice, PromptSyncConflict } from "@/lib/types/prompt-sync";
import { cn } from "@/lib/utils";

type ReviewOption = { value: PromptConflictChoice; label: string; description: string };

function ChoiceOption({ value, label, description }: ReviewOption) {
  return (
    <label
      className="flex cursor-pointer items-start gap-3 rounded-md border p-3 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid={`prompt-conflict-choice-${value}`}
    >
      <RadioGroupItem value={value} className="mt-0.5 cursor-pointer" />
      <span>
        <span className="text-sm font-medium">{label}</span>
        <span className="mt-1 block text-xs leading-5 text-muted-foreground">{description}</span>
      </span>
    </label>
  );
}

function VersionBlock({
  title,
  content,
  testId,
}: {
  title: string;
  content: string;
  testId: string;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <div className="text-xs font-medium text-muted-foreground">{title}</div>
      <pre
        className="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border bg-muted/40 p-2 text-xs"
        data-testid={testId}
      >
        {content}
      </pre>
    </div>
  );
}

// ReviewCopy is what a review list says: an intro and its two options, in
// display order.
export type ReviewCopy = { intro: string; options: ReviewOption[] };

// useConflictReviewCopy is the copy for choosing between a conflicting local
// prompt and a repository file.
export function useConflictReviewCopy(count: number): ReviewCopy {
  const { t } = useTranslation();
  return {
    intro: t("settings:promptConflictReviewIntro", { count }),
    options: [
      {
        value: "local",
        label: t("settings:promptConflictKeepLocal"),
        description: t("settings:promptConflictKeepLocalHint"),
      },
      {
        value: "repository",
        label: t("settings:promptConflictUseRepository"),
        description: t("settings:promptConflictUseRepositoryHint"),
      },
    ],
  };
}

// useRemovalReviewCopy is the copy for choosing which version a synced prompt
// keeps when Prompt Sync is removed.
export function useRemovalReviewCopy(count: number): ReviewCopy {
  const { t } = useTranslation();
  return {
    intro: t("settings:promptRemovalReviewIntro", { count }),
    options: [
      {
        value: "repository",
        label: t("settings:promptRemovalKeepRepository"),
        description: t("settings:promptRemovalKeepRepositoryHint"),
      },
      {
        value: "local",
        label: t("settings:promptRemovalRestoreLocal"),
        description: t("settings:promptRemovalRestoreLocalHint"),
      },
    ],
  };
}

type ConflictItemProps = {
  conflict: PromptSyncConflict;
  choice: PromptConflictChoice;
  options: ReviewOption[];
  onChoose: (choice: PromptConflictChoice) => void;
};

// PromptConflictItem offers the two versions of one prompt and can show both
// side by side (stacked on phones).
function PromptConflictItem({ conflict, choice, options, onChoose }: ConflictItemProps) {
  const { t } = useTranslation();
  const [comparing, setComparing] = useState(false);
  const compareId = `prompt-conflict-versions-${conflict.name}`;
  return (
    <div
      className="space-y-3 rounded-lg border border-border/70 p-3"
      data-testid="prompt-conflict-item"
      data-prompt-name={conflict.name}
    >
      <div className="min-w-0">
        <div className="text-sm font-medium">@{conflict.name}</div>
        <div className="truncate text-xs text-muted-foreground">{conflict.path}</div>
      </div>
      <RadioGroup
        value={choice}
        onValueChange={(value) => onChoose(value as PromptConflictChoice)}
        aria-label={t("settings:promptConflictChoiceLabel", { name: conflict.name })}
        className="grid gap-2 sm:grid-cols-2"
      >
        {options.map((option) => (
          <ChoiceOption key={option.value} {...option} />
        ))}
      </RadioGroup>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={() => setComparing((open) => !open)}
        aria-expanded={comparing}
        aria-controls={compareId}
        className="cursor-pointer max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        data-testid="prompt-conflict-compare"
      >
        <IconChevronDown
          className={cn("mr-1 h-4 w-4 transition-transform", comparing && "rotate-180")}
        />
        {t("settings:promptConflictCompare")}
      </Button>
      {comparing && (
        <div id={compareId} className="grid gap-3 sm:grid-cols-2">
          <VersionBlock
            title={t("settings:promptConflictLocalVersion")}
            content={conflict.local_content}
            testId="prompt-conflict-local-content"
          />
          <VersionBlock
            title={t("settings:promptConflictRepositoryVersion")}
            content={conflict.repository_content}
            testId="prompt-conflict-repository-content"
          />
        </div>
      )}
    </div>
  );
}

// PromptConflictReviewList lists every prompt under review with its choice.
// Without copy it offers the conflict choices.
export function PromptConflictReviewList({
  review,
  copy,
}: {
  review: PromptConflictReview;
  copy?: ReviewCopy;
}) {
  const conflictCopy = useConflictReviewCopy(review.conflicts.length);
  const { intro, options } = copy ?? conflictCopy;
  return (
    <div className="space-y-3" data-testid="prompt-conflict-review">
      <p className="text-sm text-muted-foreground">{intro}</p>
      {review.conflicts.map((conflict) => (
        <PromptConflictItem
          key={conflict.name}
          conflict={conflict}
          choice={review.choices[conflict.name] ?? options[0]!.value}
          options={options}
          onChoose={(choice) => review.setChoice(conflict.name, choice)}
        />
      ))}
    </div>
  );
}
