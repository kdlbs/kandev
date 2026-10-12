import { describe, expect, it } from "vitest";
import i18next from "i18next";
import type { DragStartEvent, DragOverEvent, DragEndEvent, DragCancelEvent } from "@dnd-kit/core";
import { agentProfileId } from "@/lib/types/ids";
import { profileDragAccessibility } from "./profile-drag-accessibility";

const profiles = [
  { id: agentProfileId("private-a"), name: "Alpha" },
  { id: agentProfileId("private-b"), name: "Beta" },
];
const active = { id: "private-a" };
const startEvent = { active, activatorEvent: new Event("keydown") };
const event = {
  active,
  over: { id: "private-b" },
};

describe("profile drag accessibility", () => {
  it("announces translated names and positions, including cancellation", () => {
    const accessibility = profileDragAccessibility(profiles, i18next.getFixedT("pt-pt"));
    expect(accessibility.screenReaderInstructions.draggable).toContain("barra de espaços");
    expect(accessibility.announcements.onDragStart(startEvent as DragStartEvent)).toBe(
      "Perfil Alpha selecionado. Posição 1 de 2.",
    );
    expect(accessibility.announcements.onDragOver(event as DragOverEvent)).toBe(
      "Perfil Alpha movido para a posição 2 de 2.",
    );
    expect(accessibility.announcements.onDragEnd(event as DragEndEvent)).toBe(
      "Perfil Alpha colocado na posição 2 de 2.",
    );
    expect(accessibility.announcements.onDragCancel(event as DragCancelEvent)).toBe(
      "Reordenação do perfil Alpha cancelada.",
    );
    expect(
      accessibility.announcements.onDragOver({ ...event, over: null } as DragOverEvent),
    ).toBeUndefined();
    expect(accessibility.announcements.onDragEnd({ ...event, over: null } as DragEndEvent)).toBe(
      "Reordenação do perfil Alpha cancelada.",
    );
  });
});
