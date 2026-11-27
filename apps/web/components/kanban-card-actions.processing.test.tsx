import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { t } from "@/lib/i18n";
import { KanbanCardActions } from "./kanban-card-actions";
import type { KanbanCardActionProps } from "./kanban-card-content";

afterEach(cleanup);

function actions(props: Partial<KanbanCardActionProps> = {}) {
  return (
    <StateProvider>
      <TooltipProvider>
        <KanbanCardActions
          task={{ id: "task-1", title: "Task", workflowStepId: "step-1" }}
          menuEntries={[
            { kind: "item", key: "delete", label: "Delete", onSelect: () => undefined },
          ]}
          {...props}
        />
      </TooltipProvider>
    </StateProvider>
  );
}

describe("Kanban card processing menu", () => {
  it.each(["isDeleting", "isArchiving"] as const)(
    "does not reopen a closed menu when %s starts from a confirmation dialog",
    async (processing) => {
      const view = render(actions());
      const trigger = screen.getByRole("button", { name: t("kanban:moreOptions") });
      fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
      fireEvent.click(trigger);
      fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
      await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());

      view.rerender(actions({ [processing]: true }));

      expect(screen.queryByRole("menu")).toBeNull();
      expect(document.body.style.pointerEvents).not.toBe("none");
    },
  );

  it.each(["isDeleting", "isArchiving"] as const)(
    "keeps an already open menu mounted while %s is in progress",
    async (processing) => {
      const view = render(actions());
      const trigger = screen.getByRole("button", { name: t("kanban:moreOptions") });
      fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
      fireEvent.click(trigger);
      await screen.findByRole("menu");

      view.rerender(actions({ [processing]: true }));
      fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });

      expect(screen.getByRole("menu")).toBeDefined();
    },
  );
});
