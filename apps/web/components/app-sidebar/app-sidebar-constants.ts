/** Section IDs used for both display and persistence keys in the AppSidebar. */
export const APP_SIDEBAR_SECTION_IDS = {
  tasks: "tasks",
  projects: "projects",
  agents: "agents",
  integrations: "integrations",
  settings: "settings",
} as const;

export type AppSidebarSectionId =
  (typeof APP_SIDEBAR_SECTION_IDS)[keyof typeof APP_SIDEBAR_SECTION_IDS];

export const APP_SIDEBAR_EXPANDED_WIDTH = 320;
export const APP_SIDEBAR_COLLAPSED_WIDTH = 56;

/**
 * Shared active/inactive classes for sidebar nav rows. The active state uses a
 * thin left accent bar (a `before` pseudo-element) rather than a filled
 * background — a saturated fill reads as garish against the brand theme. Rows
 * applying `SIDEBAR_ITEM_ACTIVE` get `relative` for free so the bar anchors.
 */
export const SIDEBAR_ITEM_ACTIVE =
  "relative text-foreground hover:bg-muted/60 before:absolute before:left-0 before:inset-y-1.5 before:w-[3px] before:rounded-full before:bg-primary before:content-['']";
export const SIDEBAR_ITEM_INACTIVE = "text-foreground/80 hover:bg-muted/60";
