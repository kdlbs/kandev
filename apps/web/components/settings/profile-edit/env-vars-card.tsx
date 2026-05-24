"use client";

import { useCallback, useId, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { ProfileEnvVar } from "@/lib/types/http";

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
}: {
  row: EnvVarRow;
  index: number;
  secrets: { id: string; name: string }[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
}) {
  if (row.mode === "value") {
    return (
      <Input
        value={row.value}
        onChange={(e) => onUpdate(index, "value", e.target.value)}
        placeholder="value"
        className="flex-[3] font-mono text-xs"
      />
    );
  }
  return (
    <Select value={row.secretId} onValueChange={(v) => onUpdate(index, "secretId", v)}>
      <SelectTrigger className="flex-[3] text-xs">
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
}: {
  row: EnvVarRow;
  index: number;
  secrets: { id: string; name: string }[];
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
}) {
  return (
    <li
      className="flex items-center gap-2 rounded-md border p-3"
      data-testid={`env-var-row-${index}`}
    >
      <Input
        value={row.key}
        onChange={(e) => onUpdate(index, "key", e.target.value)}
        placeholder="KEY"
        className="flex-[2] font-mono text-xs"
      />
      <Select value={row.mode} onValueChange={(v) => onUpdate(index, "mode", v)}>
        <SelectTrigger className="w-[100px] text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="value">Value</SelectItem>
          <SelectItem value="secret">Secret</SelectItem>
        </SelectContent>
      </Select>
      <ValueOrSecretInput row={row} index={index} secrets={secrets} onUpdate={onUpdate} />
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

function EnvVarAddForm({ onAdd }: { onAdd: (row: EnvVarRow) => void }) {
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

  const commit = useCallback(() => {
    const trimmedKey = draft.key.trim();
    if (trimmedKey === "") return;
    onAdd({ ...draft, key: trimmedKey });
    setDraft({ key: "", mode: "value", value: "", secretId: "" });
  }, [draft, onAdd]);

  const onEnter = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter" && draft.key.trim() !== "") {
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
        <Input
          id={valueId}
          value={draft.mode === "value" ? draft.value : ""}
          onChange={(e) => setDraft((d) => ({ ...d, value: e.target.value }))}
          placeholder={draft.mode === "value" ? "value" : "Use the trash icon, then re-add"}
          className="font-mono text-xs"
          disabled={draft.mode === "secret"}
          data-testid="env-var-new-value-input"
          onKeyDown={onEnter}
        />
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={commit}
        disabled={draft.key.trim() === ""}
        className="cursor-pointer"
        data-testid="env-var-add-button"
      >
        <IconPlus className="h-3.5 w-3.5 mr-1" />
        Add
      </Button>
    </div>
  );
}

type EnvVarsCardProps = {
  rows: EnvVarRow[];
  secrets: { id: string; name: string }[];
  onAdd: (row: EnvVarRow) => void;
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
};

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

export function EnvVarsCard({ rows, secrets, onAdd, onUpdate, onRemove }: EnvVarsCardProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Environment Variables</CardTitle>
            <CardDescription>
              Injected into the execution environment. Use Secret mode for tokens and API keys;
              literal values are stored in the profile JSON.
            </CardDescription>
          </div>
          {rows.length > 0 && (
            <span className="text-[10px] text-muted-foreground" data-testid="env-vars-count">
              {rows.length} configured
            </span>
          )}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows.length === 0 && (
          <p className="text-xs italic text-muted-foreground" data-testid="env-vars-empty">
            No environment variables configured. Add one below.
          </p>
        )}
        {rows.length > 0 && (
          <ul className="space-y-2" data-testid="env-vars-list">
            {rows.map((row, idx) => (
              <EnvVarRowComponent
                key={idx}
                row={row}
                index={idx}
                secrets={secrets}
                onUpdate={onUpdate}
                onRemove={onRemove}
              />
            ))}
          </ul>
        )}
        <EnvVarAddForm onAdd={onAdd} />
      </CardContent>
    </Card>
  );
}
