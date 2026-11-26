import { useState } from "react";
import { cleanup, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TaskFlagEditor } from "./task-flag-editor";

afterEach(cleanup);

function renderEditor(save: (note: string) => Promise<void>) {
  function Editor() {
    const [open, setOpen] = useState(true);
    return (
      <>
        <button onClick={() => setOpen(true)}>Open editor</button>
        {open && (
          <TaskFlagEditor
            savedNote="Saved text"
            readOnly={false}
            touch={false}
            onSubmit={save}
            onClose={() => setOpen(false)}
            onCloseAutoFocus={(event) => event.preventDefault()}
          />
        )}
      </>
    );
  }
  render(<Editor />);
}

const noteField = () => screen.getByRole<HTMLInputElement>("textbox", { name: "Flag note" });

it("discards the draft when cancelled and reopens the saved text", async () => {
  const save = vi.fn();
  renderEditor(save);
  fireEvent.change(noteField(), { target: { value: "An unsaved draft" } });
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(save).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Open editor" }));
  expect(noteField().value).toBe("Saved text");
});

it("keeps the draft after a failed write and allows retrying it", async () => {
  const trimmed = "Retry this text";
  const draft = `  ${trimmed}  `;
  const save = vi.fn().mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce(undefined);
  renderEditor(save);
  const saveButton = screen.getByRole<HTMLButtonElement>("button", { name: "Save" });
  fireEvent.change(noteField(), { target: { value: "   " } });
  expect(saveButton.disabled).toBe(true);
  fireEvent.change(noteField(), { target: { value: draft } });
  fireEvent.click(saveButton);
  expect(saveButton.disabled).toBe(true);
  await screen.findByRole("alert");
  expect(noteField().value).toBe(draft);
  fireEvent.click(saveButton);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(save.mock.calls).toEqual([[trimmed], [trimmed]]);
});

it("keeps oversized drafts from reaching the save handler", () => {
  const save = vi.fn();
  renderEditor(save);
  expect(noteField().getAttribute("maxlength")).toBe("1024");
  fireEvent.change(noteField(), { target: { value: "🙂".repeat(512) } });
  const saveButton = screen.getByRole<HTMLButtonElement>("button", { name: "Save" });
  expect(saveButton.disabled).toBe(false);
  fireEvent.change(noteField(), { target: { value: "🙂".repeat(512) + "x" } });
  expect(saveButton.disabled).toBe(true);
  fireEvent.submit(noteField().closest("form")!);
  expect(save).not.toHaveBeenCalled();
});
