import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FileUploadConflictDialog } from "./file-upload-conflict-dialog";
import type { ConflictChoice, PendingConflicts } from "@/hooks/use-file-upload";

afterEach(cleanup);

const KEEP_BOTH_LABEL = "Keep both";
const REPLACE_LABEL = "Replace";
const SKIP_LABEL = "Skip";
const UPLOAD_LABEL = "Upload";
const APPLY_TO_ALL = "Apply to all";
const NOT_PRESSED = "false";
const ARIA_PRESSED = "aria-pressed";
const A_TXT = "fixtures/a.txt";
const B_TXT = "fixtures/b.txt";

function pendingFor(paths: string[]): PendingConflicts {
  return {
    conflicts: paths.map((path) => ({ path, is_dir: false })),
    byDestination: new Map(paths.map((path) => [path, path])),
  };
}

function renderDialog(paths: string[]) {
  const onResolve = vi.fn();
  const onCancel = vi.fn();
  render(
    <FileUploadConflictDialog
      pending={pendingFor(paths)}
      onResolve={onResolve}
      onCancel={onCancel}
    />,
  );
  return { onResolve, onCancel };
}

function choicesFrom(onResolve: ReturnType<typeof vi.fn>): Map<string, ConflictChoice> {
  return onResolve.mock.calls[0][0] as Map<string, ConflictChoice>;
}

function clickIn(groupLabel: string, buttonLabel: string) {
  const group = screen.getByRole("group", { name: groupLabel });
  fireEvent.click(within(group).getByRole("button", { name: buttonLabel }));
}

describe("FileUploadConflictDialog", () => {
  it("renders nothing when there is no parked batch", () => {
    render(<FileUploadConflictDialog pending={null} onResolve={vi.fn()} onCancel={vi.fn()} />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("lists every conflicting path", () => {
    renderDialog([A_TXT, "fixtures/deep/b.json"]);

    expect(screen.getByText(A_TXT)).toBeTruthy();
    expect(screen.getByText("fixtures/deep/b.json")).toBeTruthy();
  });

  it("defaults to the non-destructive choice", () => {
    const { onResolve } = renderDialog([A_TXT]);

    fireEvent.click(screen.getByRole("button", { name: UPLOAD_LABEL }));

    expect(onResolve).toHaveBeenCalledTimes(1);
    expect(choicesFrom(onResolve).get(A_TXT)).toBe("keep_both");
  });

  it("applies one choice to every remaining conflict", () => {
    const { onResolve } = renderDialog([A_TXT, B_TXT, "fixtures/c.txt"]);

    clickIn(APPLY_TO_ALL, REPLACE_LABEL);
    fireEvent.click(screen.getByRole("button", { name: UPLOAD_LABEL }));

    expect([...choicesFrom(onResolve).values()]).toEqual(["replace", "replace", "replace"]);
  });

  it("keeps a per-file override after an apply-to-all", () => {
    const { onResolve } = renderDialog([A_TXT, B_TXT]);

    clickIn(APPLY_TO_ALL, SKIP_LABEL);
    clickIn(A_TXT, KEEP_BOTH_LABEL);
    fireEvent.click(screen.getByRole("button", { name: UPLOAD_LABEL }));

    const choices = choicesFrom(onResolve);
    expect(choices.get(A_TXT)).toBe("keep_both");
    expect(choices.get(B_TXT)).toBe("skip");
  });

  it("does not show an apply-to-all choice when per-file choices differ", () => {
    renderDialog([A_TXT, B_TXT]);

    clickIn(A_TXT, REPLACE_LABEL);

    const applyGroup = screen.getByRole("group", { name: APPLY_TO_ALL });
    expect(
      within(applyGroup).getByRole("button", { name: KEEP_BOTH_LABEL }).getAttribute(ARIA_PRESSED),
    ).toBe(NOT_PRESSED);
    expect(
      within(applyGroup).getByRole("button", { name: REPLACE_LABEL }).getAttribute(ARIA_PRESSED),
    ).toBe(NOT_PRESSED);
  });

  it("marks the active choice for assistive technology", () => {
    renderDialog([A_TXT]);

    const group = screen.getByRole("group", { name: A_TXT });
    expect(
      within(group).getByRole("button", { name: KEEP_BOTH_LABEL }).getAttribute(ARIA_PRESSED),
    ).toBe("true");

    clickIn(A_TXT, SKIP_LABEL);
    expect(within(group).getByRole("button", { name: SKIP_LABEL }).getAttribute(ARIA_PRESSED)).toBe(
      "true",
    );
  });

  it("reports cancellation without resolving anything", () => {
    const { onResolve, onCancel } = renderDialog([A_TXT]);

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onResolve).not.toHaveBeenCalled();
  });
});
