import type {
  TaskRemoteRepoRow,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";

export type TaskRemoteProviderReadiness = "loading" | "ready" | "unavailable" | "failed";

export type TaskRemoteProviderReadinessMap = Readonly<Record<string, TaskRemoteProviderReadiness>>;

/**
 * Rows with a provider descriptor depend on that provider's connection.
 * Anonymous pasted URLs stay provider-neutral until their normal URL
 * inspection completes.
 */
export function isPickerRemoteProviderUnavailable(
  row: TaskRemoteRepoRow,
  readiness: TaskRemoteProviderReadinessMap | undefined,
): boolean {
  if (!row.provider) return false;
  if (!readiness) return true;
  return readiness[row.provider] !== "ready";
}

export function hasUnavailablePickerRemoteProvider(
  selections: TaskRepositorySelection[],
  readiness: TaskRemoteProviderReadinessMap | undefined,
): boolean {
  return selections.some(
    (selection) =>
      selection.kind === "remote" && isPickerRemoteProviderUnavailable(selection, readiness),
  );
}
