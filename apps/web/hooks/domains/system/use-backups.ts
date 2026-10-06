"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createBackupListQueryOptions,
  invalidateBackupList,
  reloadBackupList,
  reloadBackupListAfterWrite,
  useBackupListScope,
} from "./backup-list-query";

function getErrorMessage(error: unknown): string | null {
  if (error == null) return null;
  if (error instanceof Error) return error.message;
  return String(error);
}

export function useBackups() {
  const queryClient = useQueryClient();
  const scope = useBackupListScope();
  const query = useQuery(createBackupListQueryOptions(scope.identity));
  const capturedScope = scope.captureScope();
  const reload = useCallback(
    () => reloadBackupList(queryClient, capturedScope, scope.isCurrentScope),
    [capturedScope, queryClient, scope.isCurrentScope],
  );
  const reloadAfterWrite = useCallback(
    (writerScope = capturedScope) =>
      reloadBackupListAfterWrite(queryClient, writerScope, scope.isCurrentScope),
    [capturedScope, queryClient, scope.isCurrentScope],
  );
  const invalidate = useCallback(
    (writerScope = capturedScope) => {
      if (!scope.isCurrentScope(writerScope)) return Promise.resolve();
      return invalidateBackupList(queryClient, writerScope.identity, () =>
        scope.isCurrentScope(writerScope),
      );
    },
    [capturedScope, queryClient, scope.isCurrentScope],
  );

  return {
    backups: query.data ?? [],
    loaded: query.data !== undefined,
    isLoading: query.isFetching,
    error: getErrorMessage(query.error),
    reload,
    reloadAfterWrite,
    invalidate,
    captureScope: scope.captureScope,
    isCurrentScope: scope.isCurrentScope,
    scopeIdentityKey: scope.identityKey,
    scopeGeneration: scope.generation,
  };
}
