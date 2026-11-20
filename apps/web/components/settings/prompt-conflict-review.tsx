"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import type { PromptConflictReview } from "@/hooks/domains/settings/use-prompt-conflict-review";
import type { PromptConflictChoice, PromptSyncConflict } from "@/lib/types/prompt-sync";
import { cn } from "@/lib/utils";

// ReviewCopy is what a review list says once, above its rows: an intro and
// what each version means.
export type ReviewCopy = { intro: string; localHint: string; repositoryHint: string };

// useConflictReviewCopy is the copy for choosing between a conflicting local
// prompt and a repository file.
export function useConflictReviewCopy(count: number): ReviewCopy {
  const { t } = useTranslation();
  return {
    intro: t("settings:promptConflictReviewIntro", { count }),
    localHint: t("settings:promptConflictKeepLocalHint"),
    repositoryHint: t("settings:promptConflictUseRepositoryHint"),
  };
}

// useRemovalReviewCopy is the copy for choosing which version a prompt keeps
// when Prompt Sync is removed.
export function useRemovalReviewCopy(count: number): ReviewCopy {
  const { t } = useTranslation();
  return {
    intro: t("settings:promptRemovalReviewIntro", { count }),
    localHint: t("settings:promptRemovalLocalHint"),
    repositoryHint: t("settings:promptRemovalRepositoryHint"),
  };
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

type VersionSwitchProps = {
  name: string;
  choice: PromptConflictChoice;
  onChoose: (choice: PromptConflictChoice) => void;
};

// VersionSwitch picks a prompt's version: off is the local version, on is the
// repository version. Both sides stay labelled; the chosen one is emphasized.
function VersionSwitch({ name, choice, onChoose }: VersionSwitchProps) {
  const { t } = useTranslation();
  const repository = choice === "repository";
  const id = `prompt-conflict-switch-${name}`;
  const side = (active: boolean) =>
    cn("text-xs", active ? "font-medium text-foreground" : "text-muted-foreground");
  return (
    <Label
      htmlFor={id}
      className="flex shrink-0 cursor-pointer items-center gap-2 font-normal max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
    >
      <span className={side(!repository)}>{t("settings:promptVersionLocal")}</span>
      <Switch
        id={id}
        checked={repository}
        onCheckedChange={(checked) => onChoose(checked ? "repository" : "local")}
        aria-label={t("settings:promptConflictChoiceLabel", { name })}
        className="cursor-pointer"
        data-testid="prompt-conflict-switch"
      />
      <span className={side(repository)}>{t("settings:promptVersionRepository")}</span>
    </Label>
  );
}

type ConflictRowProps = {
  conflict: PromptSyncConflict;
  choice: PromptConflictChoice;
  onChoose: (choice: PromptConflictChoice) => void;
};

// PromptConflictRow is one short row: the prompt, its version switch, and a
// toggle that shows both versions below the row.
function PromptConflictRow({ conflict, choice, onChoose }: ConflictRowProps) {
  const { t } = useTranslation();
  const [comparing, setComparing] = useState(false);
  const compareId = `prompt-conflict-versions-${conflict.name}`;
  return (
    <li className="px-3 py-2" data-testid="prompt-conflict-item" data-prompt-name={conflict.name}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">@{conflict.name}</div>
          <div className="truncate text-xs text-muted-foreground">{conflict.path}</div>
        </div>
        <VersionSwitch name={conflict.name} choice={choice} onChoose={onChoose} />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={() => setComparing((open) => !open)}
          aria-label={t("settings:promptConflictCompare")}
          aria-expanded={comparing}
          aria-controls={compareId}
          className="cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11"
          data-testid="prompt-conflict-compare"
        >
          <IconChevronDown
            className={cn("h-4 w-4 transition-transform", comparing && "rotate-180")}
          />
        </Button>
      </div>
      {comparing && (
        <div id={compareId} className="mt-2 grid gap-3 sm:grid-cols-2">
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
    </li>
  );
}

// PromptConflictReviewList explains the two versions once, then lists every
// prompt under review as a short row. Without copy it uses the conflict copy.
export function PromptConflictReviewList({
  review,
  copy,
}: {
  review: PromptConflictReview;
  copy?: ReviewCopy;
}) {
  const { t } = useTranslation();
  const conflictCopy = useConflictReviewCopy(review.conflicts.length);
  const { intro, localHint, repositoryHint } = copy ?? conflictCopy;
  return (
    <div className="space-y-3" data-testid="prompt-conflict-review">
      <p className="text-sm text-muted-foreground">{intro}</p>
      <dl
        className="grid gap-1 rounded-md bg-muted/40 p-3 text-xs"
        data-testid="prompt-conflict-legend"
      >
        <div>
          <dt className="inline font-medium">{t("settings:promptVersionLocal")}: </dt>
          <dd className="inline text-muted-foreground">{localHint}</dd>
        </div>
        <div>
          <dt className="inline font-medium">{t("settings:promptVersionRepository")}: </dt>
          <dd className="inline text-muted-foreground">{repositoryHint}</dd>
        </div>
      </dl>
      <ul className="divide-y rounded-lg border border-border/70">
        {review.conflicts.map((conflict) => (
          <PromptConflictRow
            key={conflict.name}
            conflict={conflict}
            choice={review.choices[conflict.name] ?? "local"}
            onChoose={(choice) => review.setChoice(conflict.name, choice)}
          />
        ))}
      </ul>
    </div>
  );
}
