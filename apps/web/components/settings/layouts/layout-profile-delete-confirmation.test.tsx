import { useRef, useState } from "react";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";
import { LayoutProfileDeleteConfirmation } from "./layout-profile-delete-confirmation";

afterEach(() => {
  cleanup();
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 1024 });
});

it("keeps the default-layout warning and stable trigger in the phone sheet", async () => {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
  const onConfirm = vi.fn();
  function Harness() {
    const [open, setOpen] = useState(false);
    const anchorRef = useRef<HTMLButtonElement>(null);
    return (
      <TooltipProvider>
        <LayoutProfileDeleteConfirmation
          profile={{
            id: "layout-1",
            name: "Review",
            is_default: true,
            layout: getBuiltInLayoutProfile("default").layout,
            created_at: "2026-09-10T00:00:00Z",
          }}
          isFinePointer={false}
          open={open}
          onOpenChange={setOpen}
          anchorRef={anchorRef}
          onConfirm={onConfirm}
        />
      </TooltipProvider>
    );
  }
  render(<Harness />);
  const trigger = screen.getByTestId("layout-profile-delete");
  fireEvent.click(trigger);
  const sheet = screen.getByRole("dialog");
  expect(sheet.getAttribute("data-slot")).toBe("drawer-content");
  expect(within(sheet).getByText(/built-in default layout/i)).toBeTruthy();
  expect(trigger.isConnected).toBe(true);
  fireEvent.click(within(sheet).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(document.activeElement).toBe(trigger));
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.click(trigger);
  fireEvent.click(screen.getByTestId("layout-profile-delete-confirm"));
  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
});
