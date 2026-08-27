"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";

export type SecretFormState = {
  name: string;
  value: string;
};

export const defaultFormState: SecretFormState = {
  name: "",
  value: "",
};

type SecretFormProps = {
  title: string;
  formState: SecretFormState;
  onFormChange: (patch: Partial<SecretFormState>) => void;
  onSubmit: () => void;
  onCancel: () => void;
  isValid: boolean;
  isBusy: boolean;
  submitLabel: string;
  showSubmit?: boolean;
  baselineState?: SecretFormState;
};

/** Renders the secret name/value form with submit and cancel actions. */
export function SecretForm({
  title,
  formState,
  onFormChange,
  onSubmit,
  onCancel,
  isValid,
  isBusy,
  submitLabel,
  showSubmit = true,
  baselineState,
}: SecretFormProps) {
  const { t } = useTranslation();
  const nameIsDirty = Boolean(baselineState) && formState.name.trim() !== baselineState?.name;
  const valueIsDirty = Boolean(baselineState) && formState.value !== baselineState?.value;
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 space-y-3"
      data-settings-dirty={nameIsDirty || valueIsDirty}
    >
      <div className="text-sm font-medium text-foreground">{title}</div>
      <div className="space-y-2">
        <Input
          value={formState.name}
          onChange={(e) => onFormChange({ name: e.target.value })}
          placeholder={t("settings:nameEGOpenaiProductionKey")}
          disabled={isBusy}
          data-settings-dirty={nameIsDirty}
        />
        <Textarea
          value={formState.value}
          onChange={(e) => onFormChange({ value: e.target.value })}
          placeholder={t("settings:secretValue")}
          rows={2}
          className="resize-y font-mono text-sm"
          disabled={isBusy}
          data-settings-dirty={valueIsDirty}
        />
      </div>
      <div className="flex items-center gap-2">
        {showSubmit && (
          <Button onClick={onSubmit} disabled={!isValid || isBusy} className="cursor-pointer">
            {submitLabel}
          </Button>
        )}
        <Button variant="ghost" onClick={onCancel} disabled={isBusy} className="cursor-pointer">
          {t("settings:cancel")}
        </Button>
      </div>
    </div>
  );
}
