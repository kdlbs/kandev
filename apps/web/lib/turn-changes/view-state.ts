export type TurnChangeViewState = {
  expandedKeys: string[];
  selectedFileId?: string;
  selectedFilePath?: string;
  selectedFileKind?: string;
  selectedFileOldPath?: string | null;
  selectedRepositoryChangeId?: string;
  selectedCheckoutId?: string;
  ignoreWhitespace: boolean;
};

const STORAGE_PREFIX = "kandev:turn-change-view:";
const MAX_EXPANDED_KEYS = 500;

export function readTurnChangeViewState(
  sessionId: string,
  changeSetId: string,
): TurnChangeViewState | null {
  const storage = getStorage();
  if (!storage) return null;
  try {
    const raw = storage.getItem(storageKey(sessionId, changeSetId));
    if (!raw) return null;
    const value: unknown = JSON.parse(raw);
    if (!value || typeof value !== "object") return null;
    const record = value as Record<string, unknown>;
    return {
      expandedKeys: Array.isArray(record.expandedKeys)
        ? record.expandedKeys
            .filter((key): key is string => typeof key === "string")
            .slice(0, MAX_EXPANDED_KEYS)
        : [],
      ...(typeof record.selectedFileId === "string"
        ? { selectedFileId: record.selectedFileId }
        : {}),
      ...(typeof record.selectedFilePath === "string"
        ? { selectedFilePath: record.selectedFilePath }
        : {}),
      ...(typeof record.selectedFileKind === "string"
        ? { selectedFileKind: record.selectedFileKind }
        : {}),
      ...(typeof record.selectedFileOldPath === "string" || record.selectedFileOldPath === null
        ? { selectedFileOldPath: record.selectedFileOldPath }
        : {}),
      ...(typeof record.selectedRepositoryChangeId === "string"
        ? { selectedRepositoryChangeId: record.selectedRepositoryChangeId }
        : {}),
      ...(typeof record.selectedCheckoutId === "string"
        ? { selectedCheckoutId: record.selectedCheckoutId }
        : {}),
      ignoreWhitespace: record.ignoreWhitespace === true,
    };
  } catch {
    return null;
  }
}

export function writeTurnChangeViewState(
  sessionId: string,
  changeSetId: string,
  state: TurnChangeViewState,
): void {
  const storage = getStorage();
  if (!storage) return;
  try {
    storage.setItem(
      storageKey(sessionId, changeSetId),
      JSON.stringify({
        expandedKeys: [...new Set(state.expandedKeys)].slice(0, MAX_EXPANDED_KEYS),
        selectedFileId: state.selectedFileId,
        selectedFilePath: state.selectedFilePath,
        selectedFileKind: state.selectedFileKind,
        selectedFileOldPath: state.selectedFileOldPath,
        selectedRepositoryChangeId: state.selectedRepositoryChangeId,
        selectedCheckoutId: state.selectedCheckoutId,
        ignoreWhitespace: state.ignoreWhitespace,
      }),
    );
  } catch {
    // Browser storage can be disabled or full; historical review still works in memory.
  }
}

export function updateTurnChangeViewState(
  sessionId: string,
  changeSetId: string,
  patch: Partial<TurnChangeViewState>,
): void {
  const current = readTurnChangeViewState(sessionId, changeSetId) ?? {
    expandedKeys: [],
    ignoreWhitespace: false,
  };
  writeTurnChangeViewState(sessionId, changeSetId, { ...current, ...patch });
}

function storageKey(sessionId: string, changeSetId: string): string {
  return `${STORAGE_PREFIX}${encodeURIComponent(sessionId)}:${encodeURIComponent(changeSetId)}`;
}

function getStorage(): Storage | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}
