"use client";

import { useState, useCallback } from "react";
import { IconPlus, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import { createSecret } from "@/lib/api/domains/secrets-api";
import { useAppStore } from "@/components/state-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { useTranslation } from "react-i18next";
import {
  SettingsField,
  SettingsErrorText,
  SettingsFieldLabel,
} from "@/components/settings/settings-field";
import { settingsControlClassName } from "@/components/settings/settings-control";

const NONE_VALUE = "__none__";
const CREATE_VALUE = "__create__";
const SECRET_NAME_EXAMPLE = "my-api-token";

type InlineSecretSelectProps = {
  secretId: string | null;
  onSecretIdChange: (id: string | null) => void;
  secrets: SecretListItem[];
  label?: string;
  placeholder?: string;
  isDirty?: boolean;
};

export function InlineSecretSelect({
  secretId,
  onSecretIdChange,
  secrets,
  label,
  placeholder,
  isDirty = false,
}: InlineSecretSelectProps) {
  const { t } = useTranslation();
  const [creating, setCreating] = useState(false);

  const handleValueChange = (v: string) => {
    if (v === CREATE_VALUE) {
      setCreating(true);
      return;
    }
    onSecretIdChange(v === NONE_VALUE ? null : v);
  };

  return (
    <div className="space-y-2">
      {label && <SettingsFieldLabel>{label}</SettingsFieldLabel>}
      <Select value={secretId ?? NONE_VALUE} onValueChange={handleValueChange}>
        <SelectTrigger
          className={settingsControlClassName("cursor-pointer")}
          data-settings-dirty={isDirty}
        >
          <SelectValue placeholder={placeholder ?? t("executors:selectASecret")} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={NONE_VALUE}>{t("executors:none")}</SelectItem>
          {secretId && !secrets.some((secret) => secret.id === secretId) && (
            <SelectItem value={secretId}>{t("executors:missingSecretReference")}</SelectItem>
          )}
          {secrets.map((s) => (
            <SelectItem key={s.id} value={s.id}>
              {s.name}
            </SelectItem>
          ))}
          <SelectItem value={CREATE_VALUE}>
            <span className="flex items-center gap-1">
              <IconPlus className="h-3.5 w-3.5" />
              {t("executors:createNewSecret")}
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
      {creating && (
        <InlineCreateForm
          onCreated={(item) => {
            onSecretIdChange(item.id);
            setCreating(false);
          }}
          onCancel={() => setCreating(false)}
        />
      )}
    </div>
  );
}

function InlineCreateForm({
  onCreated,
  onCancel,
}: {
  onCreated: (item: SecretListItem) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const addSecret = useAppStore((state) => state.addSecret);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSave = useCallback(async () => {
    if (!name.trim() || !value.trim()) return;
    setSaving(true);
    setError(null);
    try {
      const item = await createSecret({ name: name.trim(), value: value.trim() });
      addSecret(item);
      onCreated(item);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("executors:failedToCreateSecret"));
      setSaving(false);
    }
  }, [name, value, addSecret, onCreated]);

  return (
    <div className="rounded-md border p-3 space-y-3 bg-muted/30">
      <SettingsField label={t("executors:name")}>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("executors:eGMyApiToken", { example: SECRET_NAME_EXAMPLE })}
          className={settingsControlClassName("h-8 text-sm")}
        />
      </SettingsField>
      <SettingsField label={t("executors:value")}>
        <Textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder={t("executors:pasteYourSecretValue")}
          className={settingsControlClassName("text-sm min-h-[60px]")}
        />
      </SettingsField>
      {error && <SettingsErrorText>{error}</SettingsErrorText>}
      <div className="flex items-center gap-2 justify-end">
        <Button
          variant="outline"
          size="sm"
          onClick={onCancel}
          disabled={saving}
          className="cursor-pointer"
        >
          {t("common:cancel")}
        </Button>
        <Button
          size="sm"
          onClick={handleSave}
          disabled={!name.trim() || !value.trim() || saving}
          className="cursor-pointer"
        >
          {saving ? <IconLoader2 className="mr-1 h-3.5 w-3.5 animate-spin" /> : null}
          {t("executors:save")}
        </Button>
      </div>
    </div>
  );
}
