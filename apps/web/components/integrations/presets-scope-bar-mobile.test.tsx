import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { IconInbox } from "@tabler/icons-react";
import { IntegrationScopeBar } from "./presets-scope-bar-base";

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: true, isFinePointer: false }),
}));
afterEach(cleanup);

it("hands the real phone saved-view menu off without selecting or changing its default", async () => {
  const onSelect = vi.fn();
  const onDelete = vi.fn();
  const onDefault = vi.fn();
  render(
    <TooltipProvider>
      <IntegrationScopeBar
        testId="scope"
        savedMenuTestId="saved-menu"
        kinds={[{ value: "issue", label: "Issues" }]}
        selected={{ kind: "issue", source: "preset", id: "inbox" }}
        onSelect={onSelect}
        presetsByKind={() => [{ value: "inbox", label: "Inbox", icon: IconInbox, group: "inbox" }]}
        savedPresets={[{ id: "saved", kind: "issue", label: "My work" }]}
        onDeleteSaved={onDelete}
        onToggleSavedDefault={onDefault}
        defaultMutationPendingId={null}
        canSaveCurrent={false}
        onSaveCurrent={vi.fn()}
      />
    </TooltipProvider>,
  );
  const trigger = screen.getByTestId("saved-menu");
  fireEvent.pointerDown(trigger);
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete My work saved query" }));
  const sheet = await screen.findByRole("dialog", { name: "Delete My work?" });
  expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
  expect(screen.queryByRole("menu")).toBeNull();
  expect(document.querySelector('[data-slot="dropdown-menu-content"]')).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(onSelect).not.toHaveBeenCalled();
  expect(onDefault).not.toHaveBeenCalled();
  expect(onDelete).not.toHaveBeenCalled();
  fireEvent.pointerDown(trigger);
  fireEvent.click(screen.getByRole("menuitem", { name: "Delete My work saved query" }));
  fireEvent.click(await screen.findByRole("button", { name: "Delete My work" }));
  await waitFor(() => expect(onDelete).toHaveBeenCalledExactlyOnceWith("saved"));
});
