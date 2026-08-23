import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "./constants";

export const DEFAULT_RICH_OUTPUT_ANIMATIONS_ENABLED = true;

export function readRichOutputAnimationsEnabled(): boolean {
  const stored: unknown = getLocalStorage<boolean>(
    STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS,
    DEFAULT_RICH_OUTPUT_ANIMATIONS_ENABLED,
  );
  return typeof stored === "boolean" ? stored : DEFAULT_RICH_OUTPUT_ANIMATIONS_ENABLED;
}

export function writeRichOutputAnimationsEnabled(enabled: boolean): void {
  setLocalStorage(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS, enabled);
}
