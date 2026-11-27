import type { TFunction } from "i18next";
import type { DragStartEvent, DragOverEvent, DragEndEvent, DragCancelEvent } from "@dnd-kit/core";
import type { AgentProfile } from "@/lib/types/http";

export function profileDragAccessibility(
  profiles: Pick<AgentProfile, "id" | "name">[],
  t: TFunction,
) {
  const values = (activeId: string | number, overId: string | number = activeId) => ({
    name: profiles.find((profile) => profile.id === activeId)?.name ?? "",
    position: profiles.findIndex((profile) => profile.id === overId) + 1,
    total: profiles.length,
  });
  return {
    screenReaderInstructions: { draggable: t("agents:profileDragInstructions") },
    announcements: {
      onDragStart: ({ active }: DragStartEvent) => t("agents:profileDragStart", values(active.id)),
      onDragOver: ({ active, over }: DragOverEvent) =>
        over ? t("agents:profileDragOver", values(active.id, over.id)) : undefined,
      onDragEnd: ({ active, over }: DragEndEvent) =>
        over
          ? t("agents:profileDragEnd", values(active.id, over.id))
          : t("agents:profileDragCancel", values(active.id)),
      onDragCancel: ({ active }: DragCancelEvent) =>
        t("agents:profileDragCancel", values(active.id)),
    },
  };
}
