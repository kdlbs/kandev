import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { MobileTaskSearchAction } from "./mobile-task-search-action";
const openMode = vi.hoisted(() => vi.fn());
vi.mock("@/lib/commands/command-registry", () => ({ useCommandPanelOpen: () => ({ openMode }) }));

it("closes navigation before opening the Tasks scope", async () => {
  const closeMenu = vi.fn();
  render(<MobileTaskSearchAction closeMenu={closeMenu} />);
  fireEvent.click(screen.getByRole("button", { name: "Search tasks" }));
  expect(closeMenu).toHaveBeenCalledOnce();
  await waitFor(() => expect(openMode).toHaveBeenCalledWith("search-tasks"));
  expect(closeMenu.mock.invocationCallOrder[0]).toBeLessThan(openMode.mock.invocationCallOrder[0]);
});
