import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "./constants";

export const DEFAULT_CHAT_ANIMATIONS_ENABLED = true;

export function readChatAnimationsEnabled(): boolean {
  const stored: unknown = getLocalStorage<boolean>(
    STORAGE_KEYS.CHAT_ANIMATIONS,
    DEFAULT_CHAT_ANIMATIONS_ENABLED,
  );
  return typeof stored === "boolean" ? stored : DEFAULT_CHAT_ANIMATIONS_ENABLED;
}

export function writeChatAnimationsEnabled(enabled: boolean): void {
  setLocalStorage(STORAGE_KEYS.CHAT_ANIMATIONS, enabled);
}
