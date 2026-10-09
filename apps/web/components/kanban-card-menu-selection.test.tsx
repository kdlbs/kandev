import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { KanbanCardDropdownMenuItems, type KanbanCardMenuEntry } from "./kanban-card-menu-items";

afterEach(cleanup);

function renderMenu(entries: KanbanCardMenuEntry[], onItemSelect: () => void) {
  return render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <KanbanCardDropdownMenuItems entries={entries} onItemSelect={onItemSelect} />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
}

describe("Kanban menu selection handoff", () => {
  it.each(["click", "keyboard"])("hands off the modal before the %s action", (mode) => {
    const outcome: string[] = [];
    renderMenu(
      [{ kind: "item", key: "delete", label: "Delete", onSelect: () => outcome.push("action") }],
      () => outcome.push("handoff"),
    );
    const item = screen.getByRole("menuitem", { name: "Delete" });
    if (mode === "click") fireEvent.click(item);
    else fireEvent.keyDown(item, { key: "Enter" });
    expect(outcome).toEqual(["handoff", "action"]);
  });

  it("does not hand off or execute a disabled action", () => {
    const action = vi.fn();
    const handoff = vi.fn();
    renderMenu(
      [{ kind: "item", key: "delete", label: "Delete", disabled: true, onSelect: action }],
      handoff,
    );
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    expect(handoff).not.toHaveBeenCalled();
    expect(action).not.toHaveBeenCalled();
  });
});
