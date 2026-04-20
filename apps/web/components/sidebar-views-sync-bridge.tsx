"use client";

import { useSidebarViewsSync } from "@/hooks/use-sidebar-views-sync";

/** Mounts the sidebar views sync hook inside the ToastProvider tree. */
export function SidebarViewsSyncBridge() {
  useSidebarViewsSync();
  return null;
}
