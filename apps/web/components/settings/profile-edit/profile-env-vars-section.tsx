"use client";

import { useCallback, useState } from "react";
import { useSecrets } from "@/hooks/domains/settings/use-secrets";
import type { AgentProfile, ProfileEnvVar } from "@/lib/types/http";
import {
  EnvVarsCard,
  envVarsToRows,
  rowsToEnvVars,
  type EnvVarRow,
} from "@/components/settings/profile-edit/env-vars-card";

export function areEnvVarsEqual(a?: ProfileEnvVar[], b?: ProfileEnvVar[]): boolean {
  const left = a ?? [];
  const right = b ?? [];
  if (left.length !== right.length) return false;
  return left.every(
    (ev, i) =>
      ev.key === right[i]?.key &&
      (ev.value ?? "") === (right[i]?.value ?? "") &&
      (ev.secret_id ?? "") === (right[i]?.secret_id ?? ""),
  );
}

type ProfileEnvVarsEditorProps = {
  envVars?: ProfileEnvVar[];
  secrets: { id: string; name: string }[];
  onChange: (envVars: ProfileEnvVar[]) => void;
};

export function ProfileEnvVarsEditor({ envVars, secrets, onChange }: ProfileEnvVarsEditorProps) {
  // `synced` is what we've acknowledged from the parent (either via the prop
  // or our own last emission). When the prop diverges from it we re-seed
  // local row state; that's how external prop resets propagate without
  // wiping in-progress draft rows on every echo.
  const [synced, setSynced] = useState<ProfileEnvVar[]>(envVars ?? []);
  const [rows, setRows] = useState<EnvVarRow[]>(() => envVarsToRows(envVars));

  const incoming = envVars ?? [];
  if (!areEnvVarsEqual(incoming, synced)) {
    setSynced(incoming);
    setRows(envVarsToRows(incoming));
  }

  const commit = useCallback(
    (next: EnvVarRow[]) => {
      setRows(next);
      const cleaned = rowsToEnvVars(next);
      if (areEnvVarsEqual(cleaned, synced)) return;
      setSynced(cleaned);
      onChange(cleaned);
    },
    [synced, onChange],
  );

  const handleAdd = useCallback(
    (row: EnvVarRow) => {
      commit([...rows, row]);
    },
    [rows, commit],
  );

  const handleUpdate = useCallback(
    (index: number, field: keyof EnvVarRow, val: string) => {
      commit(rows.map((row, i) => (i === index ? { ...row, [field]: val } : row)));
    },
    [rows, commit],
  );

  const handleRemove = useCallback(
    (index: number) => {
      commit(rows.filter((_, i) => i !== index));
    },
    [rows, commit],
  );

  return (
    <EnvVarsCard
      rows={rows}
      secrets={secrets}
      onAdd={handleAdd}
      onUpdate={handleUpdate}
      onRemove={handleRemove}
    />
  );
}

type ProfileEnvVarsSectionProps = {
  envVars?: ProfileEnvVar[];
  onChange: (patch: Partial<AgentProfile>) => void;
};

export function ProfileEnvVarsSection({ envVars, onChange }: ProfileEnvVarsSectionProps) {
  const { items: secrets } = useSecrets();
  const handleChange = useCallback(
    (next: ProfileEnvVar[]) => onChange({ envVars: next }),
    [onChange],
  );

  return <ProfileEnvVarsEditor envVars={envVars} secrets={secrets} onChange={handleChange} />;
}
