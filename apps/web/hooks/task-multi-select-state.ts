export type MultiSelectOccurrence = { key: string; taskId: string };

export type MultiSelectState = {
  selectedIds: Set<string>;
  isMultiSelectEnabled: boolean;
  isDeleting: boolean;
  isArchiving: boolean;
  /**
   * The task that anchors a shift-click range selection — the last task the
   * user toggled/range-selected. `null` when there is no active anchor.
   */
  anchorId: string | null;
  occurrenceKeysByTaskId?: ReadonlyMap<string, string>;
};

export type MultiSelectAction =
  | { type: "reset" }
  | { type: "toggle_select"; taskId: string; occurrenceKey?: string }
  | { type: "select_range"; taskId: string; orderedIds: string[] }
  | {
      type: "select_occurrence_range";
      taskId: string;
      occurrenceKey?: string;
      orderedOccurrences: MultiSelectOccurrence[];
    }
  | { type: "set_selected"; ids: Set<string> }
  | { type: "set_enabled"; value: boolean }
  | { type: "set_deleting"; value: boolean }
  | { type: "set_archiving"; value: boolean };

export const INITIAL_STATE: MultiSelectState = {
  selectedIds: new Set(),
  isMultiSelectEnabled: false,
  isDeleting: false,
  isArchiving: false,
  anchorId: null,
};

/**
 * Pick a valid range anchor after the selection set is replaced wholesale: keep
 * the existing anchor if it survived, otherwise fall back to any remaining id
 * (or null when the selection is now empty).
 */
function realignAnchor(state: MultiSelectState, ids: Set<string>): string | null {
  if (ids.size === 0) return null;
  if (state.anchorId && ids.has(state.anchorId)) return state.anchorId;
  return ids.values().next().value ?? null;
}

/**
 * Union-select every id from the anchor to `taskId` (inclusive) within
 * `orderedIds`. When there is no valid anchor in `orderedIds` (first shift
 * click, or anchor lives in a different column), fall back to union-selecting
 * just `taskId` — the previous selection is preserved — and make it the new
 * anchor.
 */
function applyRangeSelect(
  state: MultiSelectState,
  taskId: string,
  orderedIds: string[],
): MultiSelectState {
  const anchor = state.anchorId;
  const anchorIdx = anchor ? orderedIds.indexOf(anchor) : -1;
  const targetIdx = orderedIds.indexOf(taskId);
  if (anchorIdx === -1 || targetIdx === -1) {
    const next = new Set(state.selectedIds);
    next.add(taskId);
    return { ...state, selectedIds: next, anchorId: taskId };
  }
  const [lo, hi] = anchorIdx < targetIdx ? [anchorIdx, targetIdx] : [targetIdx, anchorIdx];
  const next = new Set(state.selectedIds);
  for (let i = lo; i <= hi; i++) next.add(orderedIds[i]);
  return { ...state, selectedIds: next };
}

function occurrenceIndex(rows: MultiSelectOccurrence[], taskId: string, key?: string) {
  const index = rows.findIndex((row) => row.taskId === taskId && (!key || row.key === key));
  if (key || index < 0) return index;
  return rows.some((row, other) => other !== index && row.taskId === taskId) ? -1 : index;
}

function applyOccurrenceRangeSelect(
  state: MultiSelectState,
  taskId: string,
  rows: MultiSelectOccurrence[],
  occurrenceKey?: string,
): MultiSelectState {
  const origins = new Map(state.occurrenceKeysByTaskId);
  const anchorIndex = state.anchorId
    ? occurrenceIndex(rows, state.anchorId, origins.get(state.anchorId))
    : -1;
  const targetIndex = occurrenceIndex(rows, taskId, occurrenceKey);
  const next = new Set(state.selectedIds);
  if (anchorIndex < 0 || targetIndex < 0) {
    next.add(taskId);
    const key = occurrenceKey ?? rows[targetIndex]?.key;
    if (key) origins.set(taskId, key);
    return { ...state, selectedIds: next, anchorId: taskId, occurrenceKeysByTaskId: origins };
  }
  const lo = Math.min(anchorIndex, targetIndex);
  const hi = Math.max(anchorIndex, targetIndex);
  for (let index = lo; index <= hi; index++) {
    const row = rows[index];
    next.add(row.taskId);
    if (!origins.has(row.taskId)) origins.set(row.taskId, row.key);
  }
  return { ...state, selectedIds: next, occurrenceKeysByTaskId: origins };
}

function toggleSelection(
  state: MultiSelectState,
  action: Extract<MultiSelectAction, { type: "toggle_select" }>,
): MultiSelectState {
  const next = new Set(state.selectedIds);
  const added = !next.has(action.taskId);
  if (added) next.add(action.taskId);
  else next.delete(action.taskId);
  const anchorId = added ? action.taskId : realignAnchor(state, next);
  const origins =
    state.occurrenceKeysByTaskId || action.occurrenceKey
      ? new Map(state.occurrenceKeysByTaskId)
      : undefined;
  if (added && action.occurrenceKey) origins?.set(action.taskId, action.occurrenceKey);
  else origins?.delete(action.taskId);
  return {
    ...state,
    selectedIds: next,
    anchorId,
    ...(origins ? { occurrenceKeysByTaskId: origins } : {}),
  };
}

export function multiSelectReducer(
  state: MultiSelectState,
  action: MultiSelectAction,
): MultiSelectState {
  switch (action.type) {
    case "reset":
      return INITIAL_STATE;
    case "toggle_select":
      return toggleSelection(state, action);
    case "select_range":
      return applyRangeSelect(state, action.taskId, action.orderedIds);
    case "select_occurrence_range":
      return applyOccurrenceRangeSelect(
        state,
        action.taskId,
        action.orderedOccurrences,
        action.occurrenceKey,
      );
    case "set_selected": {
      const anchorId = realignAnchor(state, action.ids);
      const origins = state.occurrenceKeysByTaskId
        ? new Map([...state.occurrenceKeysByTaskId].filter(([id]) => action.ids.has(id)))
        : undefined;
      return {
        ...state,
        selectedIds: action.ids,
        anchorId,
        ...(origins ? { occurrenceKeysByTaskId: origins } : {}),
      };
    }
    case "set_enabled":
      return { ...state, isMultiSelectEnabled: action.value };
    case "set_deleting":
      return { ...state, isDeleting: action.value };
    case "set_archiving":
      return { ...state, isArchiving: action.value };
  }
}
