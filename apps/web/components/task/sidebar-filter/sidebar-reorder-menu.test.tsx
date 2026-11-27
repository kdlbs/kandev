import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { SidebarReorderMenu } from "./sidebar-reorder-menu";

afterEach(cleanup);

it("offers adjacent moves without changing order when opened and returns focus after a move", async () => {
  const onMove = vi.fn();
  render(
    <SidebarReorderMenu
      label="Repository"
      position={2}
      count={3}
      onMove={onMove}
      isDrawerLayout={false}
      testId="sidebar-reorder"
    />,
  );
  const trigger = screen.getByTestId("sidebar-reorder");

  trigger.focus();
  fireEvent.pointerDown(trigger, { button: 0, pointerType: "mouse" });
  expect(onMove).not.toHaveBeenCalled();
  fireEvent.click(screen.getByTestId("sidebar-reorder-move-down"));

  expect(onMove).toHaveBeenCalledOnce();
  expect(onMove).toHaveBeenCalledWith(1);
  await waitFor(() => expect(document.activeElement).toBe(trigger));
});

it("disables moves at both list boundaries", () => {
  render(
    <SidebarReorderMenu
      label="Only rule"
      position={1}
      count={1}
      onMove={vi.fn()}
      isDrawerLayout
      testId="single-rule-reorder"
    />,
  );
  fireEvent.pointerDown(screen.getByTestId("single-rule-reorder"), {
    button: 0,
    pointerType: "mouse",
  });

  expect(screen.getByTestId("single-rule-reorder-move-up").getAttribute("aria-disabled")).toBe(
    "true",
  );
  expect(screen.getByTestId("single-rule-reorder-move-down").getAttribute("aria-disabled")).toBe(
    "true",
  );
  expect(screen.getByTestId("single-rule-reorder").className).toContain("size-11");
});
