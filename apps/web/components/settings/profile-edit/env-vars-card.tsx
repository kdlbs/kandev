"use client";

import { useCallback, useId, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { ProfileEnvVar } from "@/lib/types/http";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsFieldLabel } from "@/components/settings/settings-typography";
import {
  settingsActionClassName,
  settingsControlClassName,
} from "@/components/settings/settings-control";
import { useTranslation } from "react-i18next";
import type { SecretListItem } from "@/lib/types/http-secrets";

export type EnvVarRow = {
  key: string;
  mode: "value" | "secret";
  value: string;
  secretId: string;
};

export function envVarsToRows(envVars?: ProfileEnvVar[]): EnvVarRow[] {
  if (!envVars || envVars.length === 0) return [];
  return envVars.map((ev) => ({
    key: ev.key,
    mode: ev.secret_id ? "secret" : "value",
    value: ev.value ?? "",
    secretId: ev.secret_id ?? "",
  }));
}

export function rowsToEnvVars(rows: EnvVarRow[]): ProfileEnvVar[] {
  return rows
    .filter((r) => r.key.trim())
    .map((r) => {
      if (r.mode === "secret" && r.secretId) {
        return { key: r.key.trim(), secret_id: r.secretId };
      }
      return { key: r.key.trim(), value: r.value };
    });
}

function ValueOrSecretInput({
  row,
  index,
  secrets,
  onUpdate,
  baselineRow,
}: {
  row: EnvVarRow;
  index: number;
  secrets: SecretListItem[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  baselineRow?: EnvVarRow;
}) {
  const { t } = useTranslation();
  const hasMissingSecret =
    Boolean(row.secretId) && !secrets.some((secret) => secret.id === row.secretId);
  if (row.mode === "value") {
    return (
      <Input
        value={row.value}
        onChange={(e) => onUpdate(index, "value", e.target.value)}
        placeholder="value"
        className="flex-[3] font-mono text-xs"
        data-settings-dirty={!baselineRow || row.value !== baselineRow.value}
      />
    );
  }
  return (
    <Select value={row.secretId} onValueChange={(v) => onUpdate(index, "secretId", v)}>
      <SelectTrigger
        className={settingsControlClassName("flex-[3]")}
        data-settings-dirty={!baselineRow || row.secretId !== baselineRow.secretId}
      >
        <SelectValue placeholder={t("executors:selectSecret")} />
      </SelectTrigger>
      <SelectContent>
        {hasMissingSecret && (
          <SelectItem value={row.secretId}>{t("executors:missingSecretReference")}</SelectItem>
        )}
        {secrets.map((s) => (
          <SelectItem key={s.id} value={s.id}>
            {s.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function EnvVarRowComponent({
  row,
  index,
  secrets,
  onUpdate,
  onRemove,
  baselineRow,
}: {
  row: EnvVarRow;
  index: number;
  secrets: SecretListItem[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
  baselineRow?: EnvVarRow;
}) {
  const { t } = useTranslation();
  return (
    <li
      className="flex items-center gap-2"
      data-testid={`env-var-row-${index}`}
      data-settings-dirty={!baselineRow || JSON.stringify(row) !== JSON.stringify(baselineRow)}
      data-settings-dirty-level="container"
    >
      <Input
        value={row.key}
        onChange={(e) => onUpdate(index, "key", e.target.value)}
        placeholder="KEY"
        className="flex-[2] font-mono text-xs"
        data-settings-dirty={!baselineRow || row.key !== baselineRow.key}
      />
      <Select value={row.mode} onValueChange={(v) => onUpdate(index, "mode", v)}>
        <SelectTrigger
          className={settingsControlClassName("w-[100px]")}
          data-settings-dirty={!baselineRow || row.mode !== baselineRow.mode}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="value">{t("executors:value")}</SelectItem>
          <SelectItem value="secret">{t("executors:secret")}</SelectItem>
        </SelectContent>
      </Select>
      <ValueOrSecretInput
        row={row}
        baselineRow={baselineRow}
        index={index}
        secrets={secrets}
        onUpdate={onUpdate}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={() => onRemove(index)}
        className="h-8 w-8 shrink-0 cursor-pointer"
        data-testid={`env-var-remove-${index}`}
        aria-label={
          row.key ? t("executors:removeEnvVarNamed", { key: row.key }) : t("executors:removeEnvVar")
        }
      >
        <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
      </Button>
    </li>
  );
}

function DraftValueInput({
  draft,
  valueId,
  secrets,
  onEnter,
  setDraft,
}: {
  draft: EnvVarRow;
  valueId: string;
  secrets: SecretListItem[];
  onEnter: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  setDraft: React.Dispatch<React.SetStateAction<EnvVarRow>>;
}) {
  const { t } = useTranslation();
  if (draft.mode === "value") {
    return (
      <Input
        id={valueId}
        value={draft.value}
        onChange={(e) => setDraft((d) => ({ ...d, value: e.target.value }))}
        placeholder="value"
        className="font-mono text-xs"
        data-testid="env-var-new-value-input"
        onKeyDown={onEnter}
      />
    );
  }
  return (
    <Select value={draft.secretId} onValueChange={(v) => setDraft((d) => ({ ...d, secretId: v }))}>
      <SelectTrigger
        id={valueId}
        className={settingsControlClassName()}
        data-testid="env-var-new-secret-select"
      >
        <SelectValue placeholder={t("executors:selectSecret")} />
      </SelectTrigger>
      <SelectContent>
        {draft.secretId && !secrets.some((secret) => secret.id === draft.secretId) && (
          <SelectItem value={draft.secretId}>{t("executors:missingSecretReference")}</SelectItem>
        )}
        {secrets.map((s) => (
          <SelectItem key={s.id} value={s.id}>
            {s.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function EnvVarAddForm({
  onAdd,
  secrets,
}: {
  onAdd: (row: EnvVarRow) => void;
  secrets: SecretListItem[];
}) {
  const { t } = useTranslation();
  const uid = useId();
  const keyId = `${uid}-key`;
  const modeId = `${uid}-mode`;
  const valueId = `${uid}-value`;
  const [draft, setDraft] = useState<EnvVarRow>({
    key: "",
    mode: "value",
    value: "",
    secretId: "",
  });

  const isAddDisabled =
    draft.key.trim() === "" || (draft.mode === "secret" && draft.secretId === "");

  const commit = useCallback(() => {
    if (isAddDisabled) return;
    onAdd({ ...draft, key: draft.key.trim() });
    setDraft({ key: "", mode: "value", value: "", secretId: "" });
  }, [draft, isAddDisabled, onAdd]);

  const onEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && !isAddDisabled) {
      e.preventDefault();
      commit();
    }
  };

  return (
    <div className="flex flex-col gap-2 md:flex-row md:items-end">
      <div className="flex-[2] space-y-1">
        <SettingsFieldLabel htmlFor={keyId}>{t("executors:key")}</SettingsFieldLabel>
        <Input
          id={keyId}
          value={draft.key}
          onChange={(e) => setDraft((d) => ({ ...d, key: e.target.value }))}
          placeholder="KEY"
          className="font-mono text-xs"
          data-testid="env-var-new-key-input"
          onKeyDown={onEnter}
        />
      </div>
      <div className="space-y-1">
        <SettingsFieldLabel htmlFor={modeId}>{t("executors:mode")}</SettingsFieldLabel>
        <Select
          value={draft.mode}
          onValueChange={(v) =>
            setDraft((d) => ({ ...d, mode: v as "value" | "secret", value: "", secretId: "" }))
          }
        >
          <SelectTrigger id={modeId} className={settingsControlClassName("w-[100px]")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="value">{t("executors:value")}</SelectItem>
            <SelectItem value="secret">{t("executors:secret")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="flex-[3] space-y-1">
        <SettingsFieldLabel htmlFor={valueId}>
          {draft.mode === "value" ? t("executors:value") : t("executors:secret")}
        </SettingsFieldLabel>
        <DraftValueInput
          draft={draft}
          valueId={valueId}
          secrets={secrets}
          onEnter={onEnter}
          setDraft={setDraft}
        />
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={commit}
        disabled={isAddDisabled}
        className={settingsActionClassName("cursor-pointer")}
        data-testid="env-var-add-button"
      >
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        {t("executors:add")}
      </Button>
    </div>
  );
}

type EnvVarsFieldProps = {
  rows: EnvVarRow[];
  baselineRows?: EnvVarRow[];
  secrets: SecretListItem[];
  onAdd: (row: EnvVarRow) => void;
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
  discoveryTargetId?: string;
};

function EnvVarsFieldBody({
  rows,
  baselineRows,
  secrets,
  onAdd,
  onUpdate,
  onRemove,
}: EnvVarsFieldProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3" data-testid="env-vars-field">
      {rows.length === 0 ? (
        <p className="text-xs italic text-muted-foreground" data-testid="env-vars-empty">
          {t("executors:noEnvironmentVariablesConfiguredAddOne")}
        </p>
      ) : (
        <ul className="space-y-2" data-testid="env-vars-list">
          {rows.map((row, idx) => (
            <EnvVarRowComponent
              key={idx}
              row={row}
              baselineRow={baselineRows?.[idx]}
              index={idx}
              secrets={secrets}
              onUpdate={onUpdate}
              onRemove={onRemove}
            />
          ))}
        </ul>
      )}
      <EnvVarAddForm onAdd={onAdd} secrets={secrets} />
    </div>
  );
}

export function useEnvVarRows(initialEnvVars?: ProfileEnvVar[]) {
  const [envVarRows, setEnvVarRows] = useState<EnvVarRow[]>(() => envVarsToRows(initialEnvVars));

  const resetEnvVars = useCallback((envVars?: ProfileEnvVar[]) => {
    setEnvVarRows(envVarsToRows(envVars));
  }, []);

  const addEnvVar = useCallback((row: EnvVarRow) => {
    setEnvVarRows((prev) => [...prev, row]);
  }, []);

  const removeEnvVar = useCallback((index: number) => {
    setEnvVarRows((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const updateEnvVar = useCallback((index: number, field: keyof EnvVarRow, val: string) => {
    setEnvVarRows((prev) => prev.map((row, i) => (i === index ? { ...row, [field]: val } : row)));
  }, []);

  return { envVarRows, addEnvVar, removeEnvVar, updateEnvVar, resetEnvVars };
}

export function EnvVarsCard(props: EnvVarsFieldProps) {
  const { t } = useTranslation();
  const isDirty =
    props.baselineRows !== undefined &&
    JSON.stringify(rowsToEnvVars(props.rows)) !== JSON.stringify(rowsToEnvVars(props.baselineRows));
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={props.discoveryTargetId}
      data-testid="env-vars-card"
    >
      <SettingsCardHeader
        title={t("executors:environmentVariables")}
        description={t("executors:injectedIntoTheExecutionEnvironmentUse")}
        actions={
          props.rows.length > 0 ? (
            <span className="text-[10px] text-muted-foreground" data-testid="env-vars-count">
              {t("executors:envVarsConfiguredCount", { count: props.rows.length })}
            </span>
          ) : undefined
        }
      />
      <CardContent>
        <EnvVarsFieldBody {...props} />
      </CardContent>
    </SettingsCard>
  );
}
