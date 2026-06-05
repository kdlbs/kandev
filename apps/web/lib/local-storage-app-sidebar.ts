import { getLocalStorage, setLocalStorage } from "./local-storage";

// --- Unified AppSidebar collapse + section expand state (localStorage, global) ---

const APP_SIDEBAR_COLLAPSED_KEY = "kandev.appSidebar.collapsed";
const APP_SIDEBAR_SECTION_EXPANDED_KEY = "kandev.appSidebar.sectionExpanded";

export function getStoredAppSidebarCollapsed(fallback: boolean): boolean {
  return getLocalStorage(APP_SIDEBAR_COLLAPSED_KEY, fallback);
}

export function setStoredAppSidebarCollapsed(collapsed: boolean): void {
  setLocalStorage(APP_SIDEBAR_COLLAPSED_KEY, collapsed);
}

export function getStoredAppSidebarSectionExpanded(
  fallback: Record<string, boolean>,
): Record<string, boolean> {
  const raw = getLocalStorage<Record<string, boolean>>(
    APP_SIDEBAR_SECTION_EXPANDED_KEY,
    fallback,
  ) as unknown;
  if (!raw || typeof raw !== "object" || Array.isArray(raw)) return { ...fallback };
  const out: Record<string, boolean> = { ...fallback };
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (typeof key === "string" && typeof value === "boolean") {
      out[key] = value;
    }
  }
  return out;
}

export function setStoredAppSidebarSectionExpanded(map: Record<string, boolean>): void {
  setLocalStorage(APP_SIDEBAR_SECTION_EXPANDED_KEY, map);
}

const APP_SIDEBAR_WIDTH_KEY = "kandev.appSidebar.width";

export function getStoredAppSidebarWidth(fallback: number): number {
  const raw = getLocalStorage<number>(APP_SIDEBAR_WIDTH_KEY, fallback) as unknown;
  if (typeof raw !== "number" || !Number.isFinite(raw) || raw <= 0) return fallback;
  return raw;
}

export function setStoredAppSidebarWidth(width: number): void {
  setLocalStorage(APP_SIDEBAR_WIDTH_KEY, width);
}
