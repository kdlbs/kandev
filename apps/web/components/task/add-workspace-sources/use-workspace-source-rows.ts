"use client";

import { useCallback, useState } from "react";
import {
  addWorkspaceSourceRow,
  getWorkspaceSourceValidation,
  removeWorkspaceSourceRow,
  updateWorkspaceSourceRow,
  type WorkspaceSourceRow,
} from "@/components/workspace-source-picker/workspace-source-state";

export function useWorkspaceSourceRows(executorType?: string | null) {
  const [rows, setRows] = useState<WorkspaceSourceRow[]>([]);
  const [validatedRowKeys, setValidatedRowKeys] = useState<Set<string>>(() => new Set());
  const errors = getWorkspaceSourceValidation(rows);
  const visibleErrors = Object.fromEntries(
    Object.entries(errors).filter(([key]) => validatedRowKeys.has(key)),
  );

  const add = useCallback(
    (kind: NonNullable<WorkspaceSourceRow["sourceType"]>) =>
      setRows((current) => addWorkspaceSourceRow(current, kind, executorType)),
    [executorType],
  );
  const update = useCallback(
    (key: string, patch: Partial<WorkspaceSourceRow>) =>
      setRows((current) => updateWorkspaceSourceRow(current, key, patch)),
    [],
  );
  const remove = useCallback(
    (key: string) => setRows((current) => removeWorkspaceSourceRow(current, key)),
    [],
  );
  const reset = useCallback(() => {
    setRows([]);
    setValidatedRowKeys(new Set());
  }, []);
  const resetValidation = useCallback(() => setValidatedRowKeys(new Set()), []);
  const validate = useCallback(
    () => setValidatedRowKeys(new Set(rows.map((row) => row.key))),
    [rows],
  );

  return {
    rows,
    errors,
    visibleErrors,
    add,
    update,
    remove,
    reset,
    resetValidation,
    validate,
  };
}
