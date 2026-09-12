"use client";

import { useCallback, useRef, useState, type RefObject } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { RepoFilterCombobox } from "./repo-filter-combobox";
import { useTranslation } from "react-i18next";
import { createFocusReturnHandler } from "@/lib/dialog-focus-return";

type SavePresetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kind: "pr" | "issue";
  customQuery: string;
  repoFilter: string;
  repoOptions: string[];
  suggestedLabel: string;
  onSave: (label: string, repoFilter: string) => Promise<boolean>;
  focusReturnRef?: RefObject<HTMLElement | null>;
};

function SavePresetForm({
  kind,
  customQuery,
  repoFilter,
  repoOptions,
  suggestedLabel,
  onSave,
  onClose,
  pending,
}: {
  kind: "pr" | "issue";
  customQuery: string;
  repoFilter: string;
  repoOptions: string[];
  suggestedLabel: string;
  onSave: (label: string, repoFilter: string) => void;
  onClose: () => void;
  pending: boolean;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(suggestedLabel);
  const [defaultRepoFilter, setDefaultRepoFilter] = useState(repoFilter);
  const trimmed = value.trim();
  const canSubmit = trimmed.length > 0 && !pending;

  const handleSubmit = useCallback(() => {
    if (!canSubmit) return;
    onSave(trimmed, defaultRepoFilter);
  }, [canSubmit, trimmed, defaultRepoFilter, onSave]);

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("github:saveQuery")}</DialogTitle>
        <DialogDescription>
          {t("github:savePresetDescription", {
            kind: kind === "pr" ? t("github:kindPullRequest") : t("github:kindIssue"),
          })}
        </DialogDescription>
      </DialogHeader>
      <fieldset disabled={pending} className="flex min-w-0 flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="preset-label" className="text-xs">
            {t("github:name")}
          </Label>
          <Input
            id="preset-label"
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onFocus={(e) => e.target.select()}
            placeholder={t("github:eGNeedsMyReview")}
          />
        </div>
        <div className="flex flex-col gap-1.5 text-xs">
          {customQuery && (
            <div className="flex gap-2">
              <span className="text-muted-foreground shrink-0 w-16">{t("github:query")}</span>
              <code className="font-mono text-[11px] bg-muted rounded px-1.5 py-0.5 break-all">
                {customQuery}
              </code>
            </div>
          )}
        </div>
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("github:defaultRepository")}</Label>
          <RepoFilterCombobox
            repoFilter={defaultRepoFilter}
            onRepoFilterChange={setDefaultRepoFilter}
            repoOptions={repoOptions}
            ariaLabel={t("github:defaultRepository")}
            triggerClassName="border border-input bg-background px-3 text-sm hover:bg-secondary/50"
            testId="github-save-query-repo-trigger"
            dropdownTestId="github-save-query-repo-dropdown"
          />
          <p className="text-xs text-muted-foreground">
            {t("github:thisRepositoryOpensByDefaultYou")}
          </p>
        </div>
      </fieldset>
      {pending && (
        <p role="status" className="text-xs text-muted-foreground">
          {t("github:saving")}
        </p>
      )}
      <DialogFooter>
        <Button variant="outline" className="cursor-pointer" onClick={onClose} disabled={pending}>
          {t("common:cancel")}
        </Button>
        <Button className="cursor-pointer" disabled={!canSubmit} onClick={handleSubmit}>
          {t("common:save")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function SavePresetDialog({
  open,
  onOpenChange,
  kind,
  customQuery,
  repoFilter,
  repoOptions,
  suggestedLabel,
  onSave,
  focusReturnRef,
}: SavePresetDialogProps) {
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!pendingRef.current) onOpenChange(nextOpen);
    },
    [onOpenChange],
  );
  const handleClose = useCallback(() => handleOpenChange(false), [handleOpenChange]);
  const handleSave = async (label: string, repo: string) => {
    if (pendingRef.current) return;
    pendingRef.current = true;
    setPending(true);
    try {
      if (await onSave(label, repo)) onOpenChange(false);
    } finally {
      pendingRef.current = false;
      setPending(false);
    }
  };
  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-md"
        aria-busy={pending}
        onCloseAutoFocus={createFocusReturnHandler(focusReturnRef)}
      >
        {open && (
          <SavePresetForm
            kind={kind}
            customQuery={customQuery}
            repoFilter={repoFilter}
            repoOptions={repoOptions}
            suggestedLabel={suggestedLabel}
            onSave={handleSave}
            onClose={handleClose}
            pending={pending}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
