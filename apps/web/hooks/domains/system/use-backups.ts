"use client";

import { useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createBackupListQueryOptions,
  invalidateBackupList,
  reloadBackupList,
  reloadBackupListAfterWrite,
  useBackupListScope,
  type BackupListScope,
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
  const reloadForScope = useCallback(
    (writerScope: BackupListScope) =>
      reloadBackupList(queryClient, writerScope, scope.isCurrentScope),
    [queryClient, scope.isCurrentScope],
  );
  const reload = useCallback(
    () => reloadForScope(scope.captureScope()),
    [reloadForScope, scope.captureScope],
  );
  const reloadAfterWrite = useCallback(
    (writerScope?: BackupListScope) =>
      reloadBackupListAfterWrite(
        queryClient,
        writerScope ?? scope.captureScope(),
        scope.isCurrentScope,
      ),
    [queryClient, scope.captureScope, scope.isCurrentScope],
  );
  const invalidate = useCallback(
    (writerScope?: BackupListScope) => {
      const scopeToInvalidate = writerScope ?? scope.captureScope();
      if (!scope.isCurrentScope(scopeToInvalidate)) return Promise.resolve();
      return invalidateBackupList(queryClient, scopeToInvalidate.identity, () =>
        scope.isCurrentScope(scopeToInvalidate),
      );
    },
    [queryClient, scope.captureScope, scope.isCurrentScope],
  );

  return {
    backups: query.data ?? [],
    loaded: query.data !== undefined,
    isLoading: query.isFetching,
    error: getErrorMessage(query.error),
    reload,
    reloadForScope,
    reloadAfterWrite,
    invalidate,
    captureScope: scope.captureScope,
    isCurrentScope: scope.isCurrentScope,
    scopeIdentityKey: scope.identityKey,
    scopeGeneration: scope.generation,
  };
}
