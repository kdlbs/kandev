"use client";

import { useCallback, useId, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { ProfileEnvVar } from "@/lib/types/http";
import { SettingsCard } from "@/components/settings/settings-card";

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
  secrets: { id: string; name: string }[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  baselineRow?: EnvVarRow;
}) {
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
        className="flex-[3] text-xs"
        data-settings-dirty={!baselineRow || row.secretId !== baselineRow.secretId}
      >
        <SelectValue placeholder="Select secret..." />
      </SelectTrigger>
      <SelectContent>
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
  secrets: { id: string; name: string }[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
  baselineRow?: EnvVarRow;
}) {
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
          className="w-[100px] text-xs"
          data-settings-dirty={!baselineRow || row.mode !== baselineRow.mode}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="value">Value</SelectItem>
          <SelectItem value="secret">Secret</SelectItem>
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
        aria-label={`Remove ${row.key || "env var"}`}
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
  secrets: { id: string; name: string }[];
  onEnter: (e: React.KeyboardEvent<HTMLInputElement>) => void;
  setDraft: React.Dispatch<React.SetStateAction<EnvVarRow>>;
}) {
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
      <SelectTrigger id={valueId} className="text-xs" data-testid="env-var-new-secret-select">
        <SelectValue placeholder="Select secret..." />
      </SelectTrigger>
      <SelectContent>
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
  secrets: { id: string; name: string }[];
}) {
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
    <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
      <div className="flex-[2] space-y-1">
        <Label className="text-xs" htmlFor={keyId}>
          Key
        </Label>
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
        <Label className="text-xs" htmlFor={modeId}>
          Mode
        </Label>
        <Select
          value={draft.mode}
          onValueChange={(v) =>
            setDraft((d) => ({ ...d, mode: v as "value" | "secret", value: "", secretId: "" }))
          }
        >
          <SelectTrigger id={modeId} className="w-[100px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="value">Value</SelectItem>
            <SelectItem value="secret">Secret</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="flex-[3] space-y-1">
        <Label className="text-xs" htmlFor={valueId}>
          {draft.mode === "value" ? "Value" : "Secret"}
        </Label>
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
        className="cursor-pointer"
        data-testid="env-var-add-button"
      >
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        Add
      </Button>
    </div>
  );
}

type EnvVarsFieldProps = {
  rows: EnvVarRow[];
  baselineRows?: EnvVarRow[];
  secrets: { id: string; name: string }[];
  onAdd: (row: EnvVarRow) => void;
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
};

function EnvVarsFieldBody({
  rows,
  baselineRows,
  secrets,
  onAdd,
  onUpdate,
  onRemove,
}: EnvVarsFieldProps) {
  return (
    <div className="space-y-3" data-testid="env-vars-field">
      {rows.length === 0 ? (
        <p className="text-xs italic text-muted-foreground" data-testid="env-vars-empty">
          No environment variables configured. Add one below.
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

  const addEnvVar = useCallback((row: EnvVarRow) => {
    setEnvVarRows((prev) => [...prev, row]);
  }, []);

  const removeEnvVar = useCallback((index: number) => {
    setEnvVarRows((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const updateEnvVar = useCallback((index: number, field: keyof EnvVarRow, val: string) => {
    setEnvVarRows((prev) => prev.map((row, i) => (i === index ? { ...row, [field]: val } : row)));
  }, []);

  return { envVarRows, addEnvVar, removeEnvVar, updateEnvVar };
}

export function EnvVarsCard(props: EnvVarsFieldProps) {
  const isDirty =
    props.baselineRows !== undefined &&
    JSON.stringify(rowsToEnvVars(props.rows)) !== JSON.stringify(rowsToEnvVars(props.baselineRows));
  return (
    <SettingsCard isDirty={isDirty} data-testid="env-vars-card">
      <CardHeader>
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle>Environment Variables</CardTitle>
            <CardDescription>
              Injected into the execution environment. Use Secret mode for tokens and API keys;
              literal values are stored in the profile JSON.
            </CardDescription>
          </div>
          {props.rows.length > 0 && (
            <span className="text-[10px] text-muted-foreground" data-testid="env-vars-count">
              {props.rows.length} configured
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <EnvVarsFieldBody {...props} />
      </CardContent>
    </SettingsCard>
  );
}
