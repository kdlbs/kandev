import type { SidebarLayoutApi } from "@/lib/types/http-user-settings";
import { fromApiSidebarLayout } from "./layout-types";

/** Only the built-in primary action replaces the inline Tasks create control. */
export function hasVisibleNewTask(layout: SidebarLayoutApi | undefined): boolean {
  if (!layout || layout.revision === 0) return true;
  return fromApiSidebarLayout(layout).nodes.some(
    (node) => node.kind === "builtin" && node.destinationId === "new_task" && node.visible,
  );
}
