"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import type { ConflictChoice, PendingConflicts } from "@/hooks/use-file-upload";

// Persisted decisions that cross the wire, never displayed.
const KEEP_BOTH: ConflictChoice = "keep_both"; // i18n-exempt: wire value
const REPLACE: ConflictChoice = "replace"; // i18n-exempt: wire value
const SKIP: ConflictChoice = "skip"; // i18n-exempt: wire value

// Keep both leads because it is the only non-destructive option.
const CHOICES: ConflictChoice[] = [KEEP_BOTH, REPLACE, SKIP];

type FileUploadConflictDialogProps = {
  pending: PendingConflicts | null;
  onResolve: (choices: Map<string, ConflictChoice>) => void;
  onCancel: () => void;
};

function ChoiceGroup({
  labels,
  value,
  ariaLabel,
  onChange,
}: {
  labels: Record<ConflictChoice, string>;
  value?: ConflictChoice;
  ariaLabel: string;
  onChange: (choice: ConflictChoice) => void;
}) {
  return (
    <div role="group" aria-label={ariaLabel} className="flex shrink-0 gap-1">
      {CHOICES.map((choice) => (
        <Button
          key={choice}
          size="sm"
          variant={value === choice ? "default" : "outline"}
          aria-pressed={value === choice}
          className={cn(
            "min-h-11 h-8 cursor-pointer px-2 text-xs sm:min-h-8",
            choice === REPLACE && "hover:text-destructive",
          )}
          onClick={() => onChange(choice)}
        >
          {labels[choice]}
        </Button>
      ))}
    </div>
  );
}

/**
 * Collects a per-file decision for every conflicting destination before any
 * byte is uploaded.
 *
 * Keep both is the default because it is the only non-destructive option;
 * Replace is irreversible and there is no version history behind it.
 */
export function FileUploadConflictDialog({
  pending,
  onResolve,
  onCancel,
}: FileUploadConflictDialogProps) {
  const { t } = useTranslation();
  const [choices, setChoices] = useState<Map<string, ConflictChoice>>(new Map());

  const labels = useMemo<Record<ConflictChoice, string>>(
    () => ({
      keep_both: t("task:uploadConflictKeepBoth"),
      replace: t("task:uploadConflictReplace"),
      skip: t("task:uploadConflictSkip"),
    }),
    [t],
  );

  const paths = useMemo(
    () => (pending ? pending.conflicts.map((conflict) => conflict.path) : []),
    [pending],
  );

  const choiceFor = useCallback(
    (path: string): ConflictChoice => choices.get(path) ?? KEEP_BOTH,
    [choices],
  );

  const applyToAllChoice = useMemo(() => {
    if (paths.length === 0) return KEEP_BOTH;
    const first = choiceFor(paths[0]);
    return paths.every((path) => choiceFor(path) === first) ? first : undefined;
  }, [paths, choiceFor]);

  const handleConfirm = useCallback(() => {
    onResolve(new Map(paths.map((path) => [path, choices.get(path) ?? KEEP_BOTH])));
    setChoices(new Map());
  }, [onResolve, paths, choices]);

  const handleCancel = useCallback(() => {
    setChoices(new Map());
    onCancel();
  }, [onCancel]);

  if (!pending) return null;

  return (
    <Dialog open onOpenChange={(open) => !open && handleCancel()}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("task:uploadConflictTitle", { count: paths.length })}</DialogTitle>
          <DialogDescription>{t("task:uploadConflictBody")}</DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-between gap-2 border-b border-foreground/10 pb-3">
          <span className="text-xs text-muted-foreground">
            {t("task:uploadConflictApplyToAll")}
          </span>
          <ChoiceGroup
            labels={labels}
            value={applyToAllChoice}
            ariaLabel={t("task:uploadConflictApplyToAll")}
            onChange={(choice) => setChoices(new Map(paths.map((path) => [path, choice])))}
          />
        </div>

        <ul className="max-h-64 space-y-2 overflow-y-auto">
          {paths.map((path) => (
            <li key={path} className="flex items-center justify-between gap-3">
              <span className="min-w-0 flex-1 truncate font-mono text-xs" title={path}>
                {path}
              </span>
              <ChoiceGroup
                labels={labels}
                value={choiceFor(path)}
                ariaLabel={path}
                onChange={(choice) => setChoices((prev) => new Map(prev).set(path, choice))}
              />
            </li>
          ))}
        </ul>

        <DialogFooter>
          <Button
            variant="outline"
            className="min-h-11 cursor-pointer sm:min-h-9"
            onClick={handleCancel}
          >
            {t("common:cancel")}
          </Button>
          <Button
            variant="default"
            className="min-h-11 cursor-pointer sm:min-h-9"
            onClick={handleConfirm}
          >
            {t("task:uploadConflictConfirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
