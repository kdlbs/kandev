"use client";

import { useCallback, useState } from "react";
import { IconPlus, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@kandev/ui/select";
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
    <div className="flex items-start gap-2">
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
      {row.mode === "value" ? (
        <Input
          value={row.value}
          onChange={(e) => onUpdate(index, "value", e.target.value)}
          placeholder="value"
          className="flex-[3] font-mono text-xs"
        />
      ) : (
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
      )}
      <Button
        type="button"
        variant="ghost"
        size="icon"
        onClick={() => onRemove(index)}
        className="h-9 w-9 shrink-0 cursor-pointer"
      >
        <IconTrash className="h-3.5 w-3.5 text-muted-foreground" />
      </Button>
    </div>
  );
}

type EnvVarsCardProps = {
  rows: EnvVarRow[];
  secrets: { id: string; name: string }[];
  onAdd: () => void;
  onUpdate: (index: number, field: keyof EnvVarRow, val: string) => void;
  onRemove: (index: number) => void;
};

export function EnvVarsCard({ rows, secrets, onAdd, onUpdate, onRemove }: EnvVarsCardProps) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Environment Variables</CardTitle>
            <CardDescription>
              Injected into the execution environment. Variables can reference secrets for sensitive
              values.
            </CardDescription>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onAdd}
            className="cursor-pointer"
          >
            <IconPlus className="mr-1 h-3.5 w-3.5" />
            Add
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {rows.length === 0 && (
          <p className="text-sm text-muted-foreground">No environment variables configured.</p>
        )}
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
      </CardContent>
    </Card>
  );
}

export function useEnvVarRows(initialEnvVars?: ProfileEnvVar[]) {
  const [envVarRows, setEnvVarRows] = useState<EnvVarRow[]>(() => envVarsToRows(initialEnvVars));

  const addEnvVar = useCallback(() => {
    setEnvVarRows((prev) => [...prev, { key: "", mode: "value", value: "", secretId: "" }]);
  }, []);

  const removeEnvVar = useCallback((index: number) => {
    setEnvVarRows((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const updateEnvVar = useCallback((index: number, field: keyof EnvVarRow, val: string) => {
    setEnvVarRows((prev) => prev.map((row, i) => (i === index ? { ...row, [field]: val } : row)));
  }, []);

  return { envVarRows, addEnvVar, removeEnvVar, updateEnvVar };
}
