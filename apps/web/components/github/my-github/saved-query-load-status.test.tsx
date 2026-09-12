import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { SavedQueryLoadStatus } from "./saved-query-load-status";

afterEach(cleanup);

it("exposes Retry as a keyboard-accessible menu item inside desktop saved queries", async () => {
  const retry = vi.fn();
  render(
    <DropdownMenu open>
      <DropdownMenuTrigger>Saved queries</DropdownMenuTrigger>
      <DropdownMenuContent>
        <SavedQueryLoadStatus loading={false} error retry={retry} presentation="menu" />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
  fireEvent.click(await screen.findByRole("menuitem", { name: "Retry" }));
  expect(retry).toHaveBeenCalledOnce();
});
