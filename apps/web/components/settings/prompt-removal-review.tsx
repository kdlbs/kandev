"use client";

import { useTranslation } from "react-i18next";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import { PromptConflictReviewList } from "@/components/settings/prompt-conflict-review";
import type { PromptRemovalReview as Removal } from "@/hooks/domains/settings/use-prompt-sync";

// RepositoryOnlyOption is the one checkbox, off by default, that deletes the
// prompts that only came from the repository instead of keeping them.
function RepositoryOnlyOption({ removal }: { removal: Removal }) {
  const { t } = useTranslation();
  const id = "prompt-removal-delete-repository-only";
  return (
    <div
      className="space-y-1 rounded-lg border border-border/70 p-3"
      data-testid="prompt-removal-repository-only"
    >
      <div className="flex items-start gap-2">
        <Checkbox
          id={id}
          checked={removal.deleteRepositoryOnly}
          onCheckedChange={(checked) => removal.setDeleteRepositoryOnly(checked === true)}
          className="mt-0.5 cursor-pointer [@media(pointer:coarse)]:size-5"
          data-testid="prompt-removal-delete-repository-only"
        />
        <Label htmlFor={id} className="cursor-pointer text-sm leading-snug">
          {t("settings:promptRemovalDeleteRepositoryOnly", {
            count: removal.repositoryOnly.length,
          })}
        </Label>
      </div>
      <p
        className="break-words pl-6 text-xs text-foreground"
        data-testid="prompt-removal-repository-only-names"
      >
        {removal.repositoryOnly.map((name) => `@${name}`).join(", ")}
      </p>
      <p className="pl-6 text-xs text-muted-foreground">
        {t("settings:promptRemovalDeleteRepositoryOnlyHint")}
      </p>
    </div>
  );
}

// PromptRemovalReview is the removal step: the version choice for prompts
// that have both versions, and the delete option for prompts that only came
// from the repository. Each part shows only when it has prompts.
export function PromptRemovalReview({ removal }: { removal: Removal }) {
  return (
    <div className="space-y-4">
      {removal.conflicts.length > 0 && <PromptConflictReviewList review={removal} kind="removal" />}
      {removal.repositoryOnly.length > 0 && <RepositoryOnlyOption removal={removal} />}
    </div>
  );
}
