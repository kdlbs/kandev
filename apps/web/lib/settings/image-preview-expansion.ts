import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "./constants";

export function readImagePreviewExpansionEnabled(): boolean {
  const stored: unknown = getLocalStorage<boolean>(STORAGE_KEYS.IMAGE_PREVIEW_EXPANSION, true);
  return typeof stored === "boolean" ? stored : true;
}

export function writeImagePreviewExpansionEnabled(enabled: boolean): void {
  setLocalStorage(STORAGE_KEYS.IMAGE_PREVIEW_EXPANSION, enabled);
}
