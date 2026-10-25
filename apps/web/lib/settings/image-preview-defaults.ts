import { getLocalStorage, setLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "./constants";

export function readImagePreviewsExpandedByDefault(): boolean {
  const stored: unknown = getLocalStorage<boolean>(
    STORAGE_KEYS.IMAGE_PREVIEWS_EXPANDED_BY_DEFAULT,
    true,
  );
  return typeof stored === "boolean" ? stored : true;
}

export function writeImagePreviewsExpandedByDefault(enabled: boolean): void {
  setLocalStorage(STORAGE_KEYS.IMAGE_PREVIEWS_EXPANDED_BY_DEFAULT, enabled);
}
