"use client";

import { useCallback, useRef, useState } from "react";
import type { ExtraRepositoryRow } from "@/components/task-create-dialog-types";

/**
 * Manages the extra-repository rows for multi-repo task creation. The primary
 * repo lives on the form's repositoryId/branch; extras here represent the
 * 2nd, 3rd, ... rows that the user adds via "+ Add repository".
 *
 * `nextKey` increments to give each row a stable client-side key without
 * relying on array indices (which would shift on removal and break inputs).
 */
export function useExtraRepositoriesState() {
  const [extraRepositories, setExtraRepositories] = useState<ExtraRepositoryRow[]>([]);
  const nextKeyRef = useRef(0);

  const addExtraRepository = useCallback(() => {
    nextKeyRef.current += 1;
    const key = `extra-${nextKeyRef.current}`;
    setExtraRepositories((rows) => [...rows, { key, repositoryId: "", branch: "" }]);
  }, []);

  const removeExtraRepository = useCallback((key: string) => {
    setExtraRepositories((rows) => rows.filter((r) => r.key !== key));
  }, []);

  const updateExtraRepository = useCallback(
    (key: string, patch: Partial<ExtraRepositoryRow>) => {
      setExtraRepositories((rows) =>
        rows.map((r) => (r.key === key ? { ...r, ...patch } : r)),
      );
    },
    [],
  );

  const resetExtraRepositories = useCallback(() => {
    setExtraRepositories([]);
    nextKeyRef.current = 0;
  }, []);

  return {
    extraRepositories,
    addExtraRepository,
    removeExtraRepository,
    updateExtraRepository,
    resetExtraRepositories,
  };
}
