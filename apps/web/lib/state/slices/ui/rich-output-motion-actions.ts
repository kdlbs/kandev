import type { StateCreator } from "zustand";
import {
  readRichOutputAnimationsEnabled,
  writeRichOutputAnimationsEnabled,
} from "@/lib/settings/rich-output-motion";
import type { RichOutputMotionState, UISlice } from "./types";

export function loadRichOutputMotionState(): RichOutputMotionState {
  const enabled = readRichOutputAnimationsEnabled();
  return { enabled, savedEnabled: enabled };
}

type ImmerSet = Parameters<StateCreator<UISlice, [["zustand/immer", never]], [], UISlice>>[0];

export function buildRichOutputMotionActions(set: ImmerSet) {
  return {
    previewRichOutputAnimations: (enabled: boolean) =>
      set((draft) => {
        draft.richOutputMotion.enabled = enabled;
      }),
    commitRichOutputAnimations: (enabled: boolean) =>
      set((draft) => {
        draft.richOutputMotion.enabled = enabled;
        draft.richOutputMotion.savedEnabled = enabled;
        writeRichOutputAnimationsEnabled(enabled);
      }),
    restoreRichOutputAnimations: () =>
      set((draft) => {
        draft.richOutputMotion.enabled = draft.richOutputMotion.savedEnabled;
      }),
  };
}
