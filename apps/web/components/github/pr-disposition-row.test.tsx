import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { ApiError } from "@/lib/api/client";
import { patchTaskPRDisposition } from "@/lib/api/domains/github-api";
import { PRDispositionRow } from "./pr-disposition-row";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";

vi.mock("@/lib/api/domains/github-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/github-api")>();
  return {
    ...actual,
    patchTaskPRDisposition: vi.fn(),
  };
});

const PR_URL = "https://github.com/kdlbs/kandev/pull/100";
const OTHER_PR_URL = "https://github.com/kdlbs/kandev/pull/101";
const SELECT_TESTID = "pr-disposition-select";

function makePR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "assoc-1",
    task_id: "task-1",
    owner: "kdlbs",
    repo: "kandev",
    pr_number: 100,
    pr_url: PR_URL,
    pr_title: "Test PR",
    head_branch: "feat",
    base_branch: "main",
    author_login: "alice",
    state: "closed",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function renderRow(pr: TaskPR) {
  const initialState = {
    workspaces: { items: [], activeId: "ws-1" },
  } as unknown as Partial<AppState>;
  return render(
    <StateProvider initialState={initialState}>
      <PRDispositionRow pr={pr} />
    </StateProvider>,
  );
}

describe("PRDispositionRow", () => {
  beforeEach(() => {
    vi.mocked(patchTaskPRDisposition).mockReset();
  });
  afterEach(() => {
    cleanup();
  });

  it("renders for a closed, unmerged row (AC-31)", () => {
    renderRow(makePR({ state: "closed", merged_at: null }));
    expect(screen.queryByTestId("pr-disposition-row")).not.toBeNull();
  });

  it("does not render for an open row (AC-34)", () => {
    renderRow(makePR({ state: "open", merged_at: null }));
    expect(screen.queryByTestId("pr-disposition-row")).toBeNull();
  });

  it("does not render for a merged row, even if state carries a stale 'closed' value (AC-34)", () => {
    renderRow(makePR({ state: "closed", merged_at: "2026-08-12T00:00:00Z" }));
    expect(screen.queryByTestId("pr-disposition-row")).toBeNull();
  });

  it("shows the recorded value for a row that already carries a disposition (AC-33)", () => {
    renderRow(makePR({ disposition: "duplicate" }));
    expect(screen.getByTestId(SELECT_TESTID).textContent).toContain("Duplicate");
  });

  it("shows the 'no disposition recorded' placeholder when null (AC-33)", () => {
    renderRow(makePR({ disposition: null }));
    expect(screen.getByTestId(SELECT_TESTID).textContent).toContain("No disposition recorded");
  });

  it("reveals the superseded-by URL field when the stored disposition is already superseded", () => {
    renderRow(
      makePR({
        disposition: "superseded",
        disposition_superseded_by_url: OTHER_PR_URL,
      }),
    );
    const urlInput = screen.getByTestId("pr-disposition-superseded-url") as HTMLInputElement;
    expect(urlInput.value).toBe(OTHER_PR_URL);
  });

  it("surfaces the server's validation error verbatim when saving a superseded URL fails (AC-32)", async () => {
    vi.mocked(patchTaskPRDisposition).mockRejectedValue(
      new ApiError("superseded_by_url resolves to this PR's own identity", 400, {
        error: "superseded_by_url resolves to this PR's own identity",
      }),
    );
    renderRow(makePR({ disposition: "superseded", disposition_superseded_by_url: "" }));

    const urlInput = screen.getByTestId("pr-disposition-superseded-url");
    fireEvent.change(urlInput, {
      target: { value: PR_URL },
    });
    fireEvent.click(screen.getByTestId("pr-disposition-save"));

    await waitFor(() => {
      expect(screen.getByTestId("pr-disposition-error").textContent).toContain(
        "superseded_by_url resolves to this PR's own identity",
      );
    });
  });

  it("clears the error and reflects the update after a successful save", async () => {
    const updated = makePR({
      disposition: "superseded",
      disposition_superseded_by_url: OTHER_PR_URL,
      disposition_recorded_at: "2026-08-13T00:00:00Z",
    });
    vi.mocked(patchTaskPRDisposition).mockResolvedValue(updated);
    renderRow(makePR({ disposition: "superseded", disposition_superseded_by_url: "" }));

    const urlInput = screen.getByTestId("pr-disposition-superseded-url");
    fireEvent.change(urlInput, {
      target: { value: OTHER_PR_URL },
    });
    fireEvent.click(screen.getByTestId("pr-disposition-save"));

    await waitFor(() => {
      expect(patchTaskPRDisposition).toHaveBeenCalledWith("assoc-1", "ws-1", {
        disposition: "superseded",
        superseded_by_url: OTHER_PR_URL,
      });
    });
    expect(screen.queryByTestId("pr-disposition-error")).toBeNull();
  });

  it("disables the save button until a superseding URL is entered", () => {
    renderRow(makePR({ disposition: "superseded", disposition_superseded_by_url: "" }));
    const button = screen.getByTestId("pr-disposition-save") as HTMLButtonElement;
    expect(button.disabled).toBe(true);
  });
});

describe("PRDispositionRow auto-save failure handling", () => {
  beforeEach(() => {
    vi.mocked(patchTaskPRDisposition).mockReset();
  });
  afterEach(() => {
    cleanup();
  });

  it("reverts the select to the persisted value when an auto-save fails", async () => {
    vi.mocked(patchTaskPRDisposition).mockRejectedValue(
      new ApiError("workspace not found", 404, { error: "workspace not found" }),
    );
    renderRow(makePR({ disposition: null }));

    fireEvent.click(screen.getByTestId(SELECT_TESTID));
    const option = await screen.findByRole("option", { name: "Duplicate" });
    fireEvent.click(option);

    await waitFor(() => {
      expect(screen.getByTestId("pr-disposition-error").textContent).toContain(
        "workspace not found",
      );
    });
    // The write failed, so the select must fall back to the last persisted
    // value ("no disposition recorded") rather than keep showing the
    // unsaved "Duplicate" pick as if it had taken effect.
    expect(screen.getByTestId(SELECT_TESTID).textContent).toContain("No disposition recorded");
  });
});
