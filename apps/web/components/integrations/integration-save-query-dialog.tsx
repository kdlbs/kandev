"use client";

import { useCallback, useMemo, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Combobox } from "@/components/combobox";
import { useTranslation } from "react-i18next";

const ALL_REPOSITORIES = "__all__";

export type IntegrationSaveQueryDialogProps = {
  open: boolean;
  onOpenChange(open: boolean): void;
  title?: string;
  description: string;
  suggestedLabel: string;
  query?: string;
  repositoryId?: string;
  repositoryOptions?: Array<{ value: string; label: string }>;
  onSave(label: string, repositoryId: string): void | Promise<void>;
};

function SavedQueryDetails({
  query,
  repositoryOptions,
  options,
  repositoryId,
  onRepositoryChange,
}: {
  query?: string;
  repositoryOptions: Array<{ value: string; label: string }>;
  options: Array<{ value: string; label: string }>;
  repositoryId: string;
  onRepositoryChange(value: string): void;
}) {
  const { t } = useTranslation();
  return (
    <>
      {query ? (
        <div className="flex gap-2 text-xs">
          <span className="w-16 shrink-0 text-muted-foreground">{t("integrations:query")}</span>
          <code className="break-all rounded bg-muted px-1.5 py-0.5 font-mono text-[11px]">
            {query}
          </code>
        </div>
      ) : null}
      {repositoryOptions.length > 0 ? (
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs">{t("integrations:defaultRepository")}</Label>
          <Combobox
            value={repositoryId || ALL_REPOSITORIES}
            onValueChange={(value) => {
              if (value) onRepositoryChange(value === ALL_REPOSITORIES ? "" : value);
            }}
            options={options}
            ariaLabel={t("integrations:defaultRepository")}
            placeholder={t("integrations:allRepos")}
            searchPlaceholder={t("integrations:filterRepositories")}
            emptyMessage={t("integrations:noRepositoriesFound")}
            triggerClassName="h-11 border border-input bg-background px-3 py-2 text-sm hover:bg-secondary/50 md:h-9 md:py-1.5"
            testId="integration-save-query-repository-trigger"
            dropdownTestId="integration-save-query-repository-dropdown"
          />
        </div>
      ) : null}
    </>
  );
}

function SaveQueryForm({
  description,
  suggestedLabel,
  query,
  repositoryId,
  repositoryOptions = [],
  onSave,
  onClose,
}: Omit<IntegrationSaveQueryDialogProps, "open" | "onOpenChange" | "title"> & {
  onClose(): void;
}) {
  const { t } = useTranslation();
  const [label, setLabel] = useState(suggestedLabel);
  const [defaultRepositoryId, setDefaultRepositoryId] = useState(repositoryId ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const options = useMemo(
    () => [{ value: ALL_REPOSITORIES, label: t("integrations:allRepos") }, ...repositoryOptions],
    [repositoryOptions, t],
  );
  const trimmedLabel = label.trim();
  const handleSave = useCallback(async () => {
    if (!trimmedLabel || saving) return;
    setSaving(true);
    setError(null);
    try {
      await onSave(trimmedLabel, defaultRepositoryId);
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("integrations:unknownError"));
    } finally {
      setSaving(false);
    }
  }, [defaultRepositoryId, onClose, onSave, saving, t, trimmedLabel]);

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("integrations:saveQuery")}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="integration-saved-query-label" className="text-xs">
            {t("integrations:queryName")}
          </Label>
          <Input
            id="integration-saved-query-label"
            autoFocus
            value={label}
            onChange={(event) => setLabel(event.target.value)}
            onFocus={(event) => event.target.select()}
            placeholder={t("integrations:queryNamePlaceholder")}
          />
        </div>
        <SavedQueryDetails
          query={query}
          repositoryOptions={repositoryOptions}
          options={options}
          repositoryId={defaultRepositoryId}
          onRepositoryChange={setDefaultRepositoryId}
        />
        {error ? (
          <p role="alert" className="text-xs text-destructive">
            {error}
          </p>
        ) : null}
      </div>
      <DialogFooter>
        <Button variant="outline" className="h-11 cursor-pointer md:h-9" onClick={onClose}>
          {t("common:cancel")}
        </Button>
        <Button
          className="h-11 cursor-pointer md:h-9"
          disabled={!trimmedLabel || saving}
          onClick={() => void handleSave()}
        >
          {saving ? t("integrations:saving") : t("common:save")}
        </Button>
      </DialogFooter>
    </>
  );
}

export function IntegrationSaveQueryDialog({
  open,
  onOpenChange,
  title,
  ...props
}: IntegrationSaveQueryDialogProps) {
  const close = useCallback(() => onOpenChange(false), [onOpenChange]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md" aria-label={title}>
        {open ? <SaveQueryForm {...props} onClose={close} /> : null}
      </DialogContent>
    </Dialog>
  );
}
