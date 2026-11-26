import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const api = {
  listNotes: vi.fn(),
  getNote: vi.fn(),
  getNoteReferences: vi.fn(),
  createNote: vi.fn(),
  editNote: vi.fn(),
  revertNote: vi.fn(),
  deleteNote: vi.fn(),
};
vi.mock("@/lib/api/domains/coordinator-notes-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-notes-api")>();
  return {
    ...actual,
    listNotes: (...a: unknown[]) => api.listNotes(...a),
    getNote: (...a: unknown[]) => api.getNote(...a),
    getNoteReferences: (...a: unknown[]) => api.getNoteReferences(...a),
    createNote: (...a: unknown[]) => api.createNote(...a),
    editNote: (...a: unknown[]) => api.editNote(...a),
    revertNote: (...a: unknown[]) => api.revertNote(...a),
    deleteNote: (...a: unknown[]) => api.deleteNote(...a),
  };
});

import { NotesSection } from "./notes-section";

const NOTE_ID = "n1";
const ROW_ID = `note-row-${NOTE_ID}`;
const CITATION = { kind: "task", id: "t-1" };
function note(over: Record<string, unknown> = {}) {
  return {
    id: NOTE_ID,
    type: "project",
    text: "Releases ship on Tuesdays",
    citations: [CITATION],
    provenance: { kind: "human", user_id: "u" },
    version: 2,
    status: "live",
    created_at: "2026-09-12T10:00:00Z",
    expires_at: "2026-10-12T10:00:00Z",
    use_count: 0,
    ...over,
  };
}
function detail(over: Record<string, unknown> = {}) {
  const base = note(over);
  return {
    ...base,
    versions: [
      {
        note_id: NOTE_ID,
        version: 1,
        type: "project",
        text: "old",
        citations: [CITATION],
        provenance: { kind: "human" },
        created_at: "2026-09-12T09:00:00Z",
      },
      {
        note_id: NOTE_ID,
        version: 2,
        type: "project",
        text: base.text,
        citations: [CITATION],
        provenance: { kind: "human" },
        created_at: "2026-09-12T10:00:00Z",
      },
    ],
  };
}
function conflict(reason: string) {
  return new ApiError("conflict", 409, { code: "conflict", reason });
}
function renderSection(canManage = true) {
  return render(<NotesSection workspaceId="ws" coordinatorId="co" canManage={canManage} />);
}

beforeEach(() => {
  Object.values(api).forEach((m) => m.mockReset());
  api.listNotes.mockResolvedValue([note()]);
  api.getNote.mockResolvedValue(detail());
  api.getNoteReferences.mockResolvedValue([]);
});
afterEach(cleanup);

describe("NotesSection", () => {
  it("shows loading, then the list", async () => {
    renderSection();
    expect(screen.getByTestId("notes-loading")).toBeTruthy();
    expect(await screen.findByTestId(ROW_ID)).toBeTruthy();
  });

  it("distinguishes empty from error and retries", async () => {
    api.listNotes.mockResolvedValueOnce([]);
    const { unmount } = renderSection();
    expect(await screen.findByTestId("notes-empty")).toBeTruthy();
    unmount();
    api.listNotes.mockRejectedValueOnce(new Error("down")).mockResolvedValueOnce([note()]);
    renderSection();
    expect(await screen.findByTestId("notes-error")).toBeTruthy();
    fireEvent.click(screen.getByText("Retry"));
    expect(await screen.findByTestId(ROW_ID)).toBeTruthy();
  });

  it("requests deleted notes only when asked", async () => {
    renderSection();
    await screen.findByTestId(ROW_ID);
    expect(api.listNotes).toHaveBeenLastCalledWith("ws", "co", false);
    fireEvent.click(screen.getByTestId("notes-include-deleted"));
    await waitFor(() => expect(api.listNotes).toHaveBeenLastCalledWith("ws", "co", true));
  });

  it("opens a note with versions and hides write actions without permission", async () => {
    renderSection(false);
    fireEvent.click(await screen.findByTestId(ROW_ID));
    expect(await screen.findByTestId("note-version-1")).toBeTruthy();
    expect(screen.queryByTestId("note-edit")).toBeNull();
    expect(screen.queryByTestId("note-add")).toBeNull();
    expect(screen.queryByTestId("note-revert-1")).toBeNull();
  });

  it("reverts to an earlier version against the current version", async () => {
    api.revertNote.mockResolvedValue(note());
    renderSection();
    fireEvent.click(await screen.findByTestId(ROW_ID));
    fireEvent.click(await screen.findByTestId("note-revert-1"));
    await waitFor(() =>
      expect(api.revertNote).toHaveBeenCalledWith("ws", "co", NOTE_ID, {
        target_version: 1,
        version: 2,
      }),
    );
    expect(screen.queryByTestId("note-revert-2")).toBeNull();
  });

  it("asks twice before deleting", async () => {
    api.deleteNote.mockResolvedValue(note({ status: "deleted" }));
    renderSection();
    fireEvent.click(await screen.findByTestId(ROW_ID));
    fireEvent.click(await screen.findByTestId("note-delete"));
    expect(api.deleteNote).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("note-delete-confirm"));
    await waitFor(() => expect(api.deleteNote).toHaveBeenCalledWith("ws", "co", NOTE_ID, 2));
  });

  it("hides edit and revert for an expired note but keeps delete", async () => {
    api.getNote.mockResolvedValue(detail({ status: "expired" }));
    renderSection();
    fireEvent.click(await screen.findByTestId(ROW_ID));
    await screen.findByTestId("note-delete");
    expect(screen.queryByTestId("note-edit")).toBeNull();
    expect(screen.queryByTestId("note-revert-1")).toBeNull();
  });
});

describe("NotesSection writes", () => {
  it("creates a note and opens it", async () => {
    api.createNote.mockResolvedValue(note({ id: "n2" }));
    api.listNotes.mockResolvedValue([]);
    renderSection();
    await screen.findByTestId("notes-empty");
    fireEvent.click(screen.getByTestId("note-add"));
    expect((screen.getByTestId("note-save") as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByTestId("note-text"), { target: { value: "  A fact  " } });
    fireEvent.change(screen.getByLabelText("Identifier"), {
      target: { value: " t-9 " },
    });
    fireEvent.click(screen.getByTestId("note-save"));
    await waitFor(() => expect(api.createNote).toHaveBeenCalled());
    expect(api.createNote.mock.calls[0][2]).toMatchObject({
      type: "project",
      citations: [{ kind: "task", id: "t-9" }],
    });
  });

  it("keeps the draft and error when a create is rejected", async () => {
    api.createNote.mockRejectedValue(new ApiError("x", 422, { code: "credential_detected" }));
    renderSection();
    fireEvent.click(await screen.findByTestId("note-add"));
    fireEvent.change(screen.getByTestId("note-text"), { target: { value: "secret" } });
    fireEvent.change(screen.getByLabelText("Identifier"), {
      target: { value: "t-1" },
    });
    fireEvent.click(screen.getByTestId("note-save"));
    expect((await screen.findByTestId("note-editor-error")).textContent).toBe(
      "The text looks like it contains a credential. Remove it and try again.",
    );
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).value).toBe("secret");
  });

  it("keeps the draft on a version conflict and reloads only when asked", async () => {
    api.editNote.mockRejectedValue(conflict("version"));
    renderSection();
    fireEvent.click(await screen.findByTestId(ROW_ID));
    fireEvent.click(await screen.findByTestId("note-edit"));
    fireEvent.change(screen.getByTestId("note-text"), { target: { value: "my edit" } });
    fireEvent.click(screen.getByTestId("note-save"));
    expect((await screen.findByTestId("note-editor-error")).textContent).toBe(
      "Someone changed this note since you opened it. Your draft is kept. Load the latest version, then save again.",
    );
    const reads = api.getNote.mock.calls.length;
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).value).toBe("my edit");
    fireEvent.click(screen.getByTestId("note-conflict-reload"));
    await waitFor(() => expect(api.getNote.mock.calls.length).toBe(reads + 1));
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).value).toBe("my edit");
    expect(screen.queryByTestId("note-editor-error")).toBeNull();
  });

  it("freezes the draft when the note expired and offers a new note from it", async () => {
    api.editNote.mockRejectedValue(conflict("expired"));
    renderSection();
    fireEvent.click(await screen.findByTestId(ROW_ID));
    fireEvent.click(await screen.findByTestId("note-edit"));
    fireEvent.change(screen.getByTestId("note-text"), { target: { value: "kept text" } });
    fireEvent.click(screen.getByTestId("note-save"));
    await screen.findByTestId("note-conflict-create");
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).disabled).toBe(true);
    expect((screen.getByTestId("note-save") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId("note-conflict-reload")).toBeNull();
    fireEvent.click(screen.getByTestId("note-conflict-create"));
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).value).toBe("kept text");
    expect((screen.getByTestId("note-text") as HTMLTextAreaElement).disabled).toBe(false);
  });
});
