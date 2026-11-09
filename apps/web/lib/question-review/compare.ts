export type CompareMode = "toggle" | "wipe" | "side" | "onion" | "diff";

export const COMPARE_MODES: readonly CompareMode[] = ["toggle", "wipe", "side", "onion", "diff"];
export const DEFAULT_TOLERANCE = 16;
export const MAX_TOLERANCE = 64;
const NUDGE = 0.05;

export type CompareState = {
  mode: CompareMode;
  aId: string;
  bId: string;
  /** In Toggle, whether B is the visible image. */
  showB: boolean;
  /** Wipe divider position as a fraction of the stage width. */
  wipe: number;
  /** Onion-skin opacity of B. */
  opacity: number;
  tolerance: number;
};

export type CompareAction =
  | { type: "mode"; mode: CompareMode }
  | { type: "pick"; slot: "a" | "b"; id: string }
  | { type: "swap" }
  | { type: "flip" }
  | { type: "wipe"; value: number }
  | { type: "opacity"; value: number }
  | { type: "tolerance"; value: number }
  | { type: "nudge"; direction: 1 | -1 }
  | { type: "show"; id: string }
  | { type: "step"; direction: 1 | -1; ids: readonly string[] };

/** The parts of a comparison that carry over to the next segment. */
export type CompareSettings = Pick<CompareState, "mode" | "wipe" | "opacity" | "tolerance">;

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export function initialCompareState(
  imageIds: readonly string[],
  settings?: CompareSettings,
): CompareState {
  return {
    mode: settings?.mode ?? "toggle",
    aId: imageIds[0] ?? "",
    bId: imageIds[1] ?? imageIds[0] ?? "",
    showB: false,
    wipe: settings?.wipe ?? 0.5,
    opacity: settings?.opacity ?? 0.5,
    tolerance: settings?.tolerance ?? DEFAULT_TOLERANCE,
  };
}

export function compareSettings(state: CompareState): CompareSettings {
  return { mode: state.mode, wipe: state.wipe, opacity: state.opacity, tolerance: state.tolerance };
}

/** Candidates are every image except A, in segment order. */
function candidates(state: CompareState, ids: readonly string[]): string[] {
  return ids.filter((id) => id !== state.aId);
}

/** Zero-based position of B among the candidates. */
export function candidatePosition(state: CompareState, ids: readonly string[]) {
  const list = candidates(state, ids);
  return { index: list.indexOf(state.bId), total: list.length };
}

function step(state: CompareState, direction: 1 | -1, ids: readonly string[]): CompareState {
  const list = candidates(state, ids);
  const next = list[list.indexOf(state.bId) + direction];
  if (!next) return state;
  return { ...state, bId: next, showB: state.mode === "toggle" ? true : state.showB };
}

function pick(state: CompareState, slot: "a" | "b", id: string): CompareState {
  const other = slot === "a" ? state.bId : state.aId;
  if (id === other) return { ...state, aId: state.bId, bId: state.aId };
  return slot === "a" ? { ...state, aId: id } : { ...state, bId: id };
}

function nudge(state: CompareState, direction: 1 | -1): CompareState {
  if (state.mode === "wipe") return { ...state, wipe: clamp(state.wipe + direction * NUDGE, 0, 1) };
  if (state.mode === "onion") {
    return { ...state, opacity: clamp(state.opacity + direction * NUDGE, 0, 1) };
  }
  return state;
}

/** Shows one image alone in Toggle, as when a region drawn on it is selected. */
function show(state: CompareState, id: string): CompareState {
  if (id === state.aId) return { ...state, mode: "toggle", showB: false };
  if (id === state.bId) return { ...state, mode: "toggle", showB: true };
  return { ...state, mode: "toggle", aId: id, showB: false };
}

export function compareReducer(state: CompareState, action: CompareAction): CompareState {
  switch (action.type) {
    case "mode":
      return { ...state, mode: action.mode };
    case "pick":
      return pick(state, action.slot, action.id);
    case "swap":
      return { ...state, aId: state.bId, bId: state.aId };
    case "flip":
      return { ...state, showB: !state.showB };
    case "wipe":
      return { ...state, wipe: clamp(action.value, 0, 1) };
    case "opacity":
      return { ...state, opacity: clamp(action.value, 0, 1) };
    case "tolerance":
      return { ...state, tolerance: Math.round(clamp(action.value, 0, MAX_TOLERANCE)) };
    case "nudge":
      return nudge(state, action.direction);
    case "show":
      return show(state, action.id);
    case "step":
      return step(state, action.direction, action.ids);
  }
}
