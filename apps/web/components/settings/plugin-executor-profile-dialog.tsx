"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { PluginExecutorProfileFields } from "@/components/settings/plugin-executor-profile-fields";
import {
  parseExecutorProfileSchema,
  serializeExecutorProfileValues,
  validateExecutorProfileValues,
} from "@/lib/plugins/executor-profile-schema";
import {
  localizedExecutorProviderMessage,
  executorProviderRetentionText,
  executorProviderUnavailableReason,
} from "@/lib/executor-provider-display";
import { createExecutorProfile } from "@/lib/api/domains/settings-api";
import type { ExecutorProfile, ExecutorProvider } from "@/lib/types/http";
import { settingsActionClassName } from "@/components/settings/settings-control";

type PluginExecutorProfileDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  executorId: string;
  provider: ExecutorProvider | undefined;
  onSaved?: (profile: ExecutorProfile) => void;
};

function ProfileCreationFields({
  provider,
  fields,
  supported,
  name,
  values,
  errors,
  onNameChange,
  onValueChange,
}: {
  provider?: ExecutorProvider;
  fields: ReturnType<typeof parseExecutorProfileSchema>["fields"];
  supported: boolean;
  name: string;
  values: Record<string, string>;
  errors: ReturnType<typeof validateExecutorProfileValues>;
  onNameChange: (name: string) => void;
  onValueChange: (name: string, value: string) => void;
}) {
  const { t } = useTranslation();
  const unavailableReason = executorProviderUnavailableReason(provider, true, t);
  const retentionText = executorProviderRetentionText(provider, t);
  const description = localizedExecutorProviderMessage(
    provider,
    "description",
    provider?.description ?? t("executors:providerUnavailable"),
    t,
  );
  return (
    <>
      <DialogHeader className="shrink-0 text-left">
        <DialogTitle>{t("executors:newProfile")}</DialogTitle>
        <DialogDescription>{description}</DialogDescription>
      </DialogHeader>
      <div
        className="min-h-0 flex-1 space-y-5 overflow-y-auto px-1 py-2"
        data-testid="plugin-executor-profile-create-form"
      >
        {retentionText && <p className="text-sm text-muted-foreground">{retentionText}</p>}
        {unavailableReason && (
          <p
            role="status"
            className="rounded-md border border-amber-500/40 bg-amber-500/5 p-3 text-sm"
          >
            {unavailableReason}
          </p>
        )}
        {!supported && (
          <p role="alert" className="text-sm text-destructive">
            {t("executors:providerSchemaUnsupported")}
          </p>
        )}
        <div className="space-y-1.5">
          <Label htmlFor="plugin-executor-profile-name">{t("executors:name")}</Label>
          <Input
            id="plugin-executor-profile-name"
            value={name}
            disabled={!supported || Boolean(unavailableReason)}
            className="min-h-11 w-full"
            onChange={(event) => onNameChange(event.target.value)}
          />
          {!name.trim() && (
            <p className="text-sm text-destructive">{t("executors:providerProfileNameRequired")}</p>
          )}
        </div>
        {supported && (
          <PluginExecutorProfileFields
            fields={fields}
            provider={provider}
            values={values}
            errors={errors}
            disabled={Boolean(unavailableReason)}
            onValueChange={onValueChange}
          />
        )}
      </div>
    </>
  );
}

function ProfileCreationActions({
  canSave,
  saving,
  onCancel,
  onCreate,
}: {
  canSave: boolean;
  saving: boolean;
  onCancel: () => void;
  onCreate: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter className="shrink-0 border-t pt-3">
      <Button
        variant="outline"
        onClick={onCancel}
        className={settingsActionClassName("min-h-11 cursor-pointer")}
      >
        {t("common:cancel")}
      </Button>
      <Button
        onClick={onCreate}
        disabled={!canSave}
        className={settingsActionClassName("min-h-11 cursor-pointer")}
      >
        {saving ? t("executors:creating") : t("executors:createProfile")}
      </Button>
    </DialogFooter>
  );
}

export function PluginExecutorProfileDialog({
  open,
  onOpenChange,
  executorId,
  provider,
  onSaved,
}: PluginExecutorProfileDialogProps) {
  const { t } = useTranslation();
  const parsed = useMemo(
    () => parseExecutorProfileSchema(provider?.profile_schema),
    [provider?.profile_schema],
  );
  const [name, setName] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [fieldErrors, setFieldErrors] = useState<ReturnType<typeof validateExecutorProfileValues>>(
    {},
  );
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const validationErrors = validateExecutorProfileValues(parsed.fields, values);
  const unavailableReason = executorProviderUnavailableReason(provider, true, t);
  const canSave = Boolean(name.trim()) && parsed.supported && !unavailableReason && !saving;

  useEffect(() => {
    if (!open) return;
    setName("");
    setValues(Object.fromEntries(parsed.fields.map((field) => [field.name, ""])));
    setFieldErrors({});
    setError(null);
  }, [open, parsed.fields]);

  const handleSave = async () => {
    setFieldErrors(validationErrors);
    if (!canSave || Object.keys(validationErrors).length > 0) return;
    setSaving(true);
    setError(null);
    try {
      const profile = await createExecutorProfile(executorId, {
        name: name.trim(),
        config: serializeExecutorProfileValues(parsed.fields, values),
      });
      onOpenChange(false);
      onSaved?.(profile);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("executors:failedToCreateProfile"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden pb-[env(safe-area-inset-bottom,0px)] sm:max-w-2xl">
        <ProfileCreationFields
          provider={provider}
          fields={parsed.fields}
          supported={parsed.supported}
          name={name}
          values={values}
          errors={fieldErrors}
          onNameChange={setName}
          onValueChange={(field, value) => {
            setValues((current) => ({ ...current, [field]: value }));
            setFieldErrors((current) => {
              const next = { ...current };
              delete next[field];
              return next;
            });
          }}
        />
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <ProfileCreationActions
          canSave={canSave}
          saving={saving}
          onCancel={() => onOpenChange(false)}
          onCreate={() => void handleSave()}
        />
      </DialogContent>
    </Dialog>
  );
}
