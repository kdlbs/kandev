import type { StateCreator } from "zustand";
import { readChatAnimationsEnabled, writeChatAnimationsEnabled } from "@/lib/settings/chat-motion";
import type { ChatMotionState, UISlice } from "./types";

export function loadChatMotionState(): ChatMotionState {
  const enabled = readChatAnimationsEnabled();
  return { enabled, savedEnabled: enabled };
}

type ImmerSet = Parameters<StateCreator<UISlice, [["zustand/immer", never]], [], UISlice>>[0];

export function buildChatMotionActions(set: ImmerSet) {
  return {
    previewChatAnimations: (enabled: boolean) =>
      set((draft) => {
        draft.chatMotion.enabled = enabled;
      }),
    commitChatAnimations: (enabled: boolean) =>
      set((draft) => {
        draft.chatMotion.enabled = enabled;
        draft.chatMotion.savedEnabled = enabled;
        writeChatAnimationsEnabled(enabled);
      }),
    restoreChatAnimations: () =>
      set((draft) => {
        draft.chatMotion.enabled = draft.chatMotion.savedEnabled;
      }),
  };
}
