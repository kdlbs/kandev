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

export const APP_SIDEBAR_EXPANDED_WIDTH = 240;
export const APP_SIDEBAR_COLLAPSED_WIDTH = 56;
