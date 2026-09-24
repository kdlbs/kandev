import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TaskChromeDisclosure } from "./task-chrome-disclosure";

const pointer = vi.hoisted(() => ({ touch: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => pointer.touch }));

afterEach(() => {
  cleanup();
  pointer.touch = false;
});

const props = { label: "Task tools", icon: <span aria-hidden>+</span>, testId: "tools-trigger" };

it.each([false, true])("opens and dismisses the real disclosure with touch=%s", async (touch) => {
  pointer.touch = touch;
  render(
    <TaskChromeDisclosure {...props}>
      <button type="button">Change layout</button>
    </TaskChromeDisclosure>,
  );
  const trigger = screen.getByRole("button", { name: props.label });
  fireEvent.click(trigger);
  const dialog = await screen.findByRole("dialog", { name: props.label });
  expect(dialog.getAttribute("data-slot")).toBe(touch ? "drawer-content" : "popover-content");
  if (!touch) await waitFor(() => expect(document.activeElement).toBe(dialog));
  fireEvent.keyDown(dialog, { key: "Escape" });
  await waitFor(() => expect(trigger.getAttribute("aria-expanded")).toBe("false"));
  if (!touch) await waitFor(() => expect(document.activeElement).toBe(trigger));
});

it.each([false, true])("lets the parent control disclosure state with touch=%s", async (touch) => {
  pointer.touch = touch;
  const onOpenChange = vi.fn();
  const content = (open: boolean) => (
    <TaskChromeDisclosure {...props} open={open} onOpenChange={onOpenChange}>
      <button type="button">Change layout</button>
    </TaskChromeDisclosure>
  );
  const { rerender } = render(content(false));
  const trigger = screen.getByRole("button", { name: props.label });
  fireEvent.click(trigger);
  expect(onOpenChange).toHaveBeenLastCalledWith(true);
  expect(screen.queryByRole("dialog", { name: props.label })).toBeNull();

  rerender(content(true));
  const dialog = await screen.findByRole("dialog", { name: props.label });
  fireEvent.keyDown(dialog, { key: "Escape" });
  expect(onOpenChange).toHaveBeenLastCalledWith(false);
  expect(screen.getByRole("dialog", { name: props.label })).toBe(dialog);

  rerender(content(false));
  await waitFor(() => expect(trigger.getAttribute("aria-expanded")).toBe("false"));
});
