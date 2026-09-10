import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import i18n from "i18next";
import { MobileConfirmationContent } from "./mobile-confirmation-content";

afterEach(cleanup);

it("isolates confirmation mouse events from row selection owners", () => {
  const onMouseDown = vi.fn();
  render(
    <div onMouseDown={onMouseDown}>
      <MobileConfirmationContent
        title="Delete file?"
        cancelLabel="Cancel"
        confirmLabel="Delete"
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />
    </div>,
  );
  fireEvent.mouseDown(screen.getByRole("button", { name: "Cancel" }));
  expect(onMouseDown).not.toHaveBeenCalled();
});

it("updates Back with the active locale without reopening", async () => {
  render(
    <MobileConfirmationContent
      title="Delete file?"
      cancelLabel="Cancel"
      confirmLabel="Delete"
      onBack={vi.fn()}
      onCancel={vi.fn()}
      onConfirm={vi.fn()}
    />,
  );
  const back = screen.getByRole("button", { name: "Back" });
  try {
    await act(async () => {
      await i18n.changeLanguage("pt-pt");
    });
    expect(screen.getByRole("button", { name: "Voltar" })).toBe(back);
  } finally {
    await act(async () => {
      await i18n.changeLanguage("en");
    });
  }
});

it("names the target, focuses Cancel, and requires explicit action activation", () => {
  const onConfirm = vi.fn();
  render(
    <MobileConfirmationContent
      title="Delete file?"
      subject="notes.txt"
      description="This file will be deleted."
      cancelLabel="Cancel"
      confirmLabel="Delete"
      onCancel={vi.fn()}
      onConfirm={onConfirm}
    />,
  );
  const group = screen.getByRole("group", { name: "Delete file?" });
  expect(screen.getByText("notes.txt")).toBeTruthy();
  expect(group.getAttribute("aria-describedby")).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
  fireEvent.keyDown(group, { key: "Enter" });
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Delete" }));
  expect(onConfirm).toHaveBeenCalledOnce();
});

it("keeps cancellation available when the action is disabled", () => {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  render(
    <MobileConfirmationContent
      title="Archive task?"
      subject="Task A"
      cancelLabel="Cancel"
      confirmLabel="Archive"
      variant="default"
      confirmDisabled
      onCancel={onCancel}
      onConfirm={onConfirm}
    />,
  );
  const confirm = screen.getByRole("button", { name: "Archive" });
  expect(confirm.getAttribute("data-variant")).toBe("default");
  fireEvent.click(confirm);
  expect(onConfirm).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(onCancel).toHaveBeenCalledOnce();
});
