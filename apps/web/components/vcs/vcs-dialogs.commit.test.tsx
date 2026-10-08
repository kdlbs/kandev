import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import type { Window as HappyDOMWindow } from "happy-dom";
import type { StoreApi } from "zustand";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { VcsDialogsProvider, useVcsDialogs } from "./vcs-dialogs";

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: vi.fn(),
}));

const commitRequest = vi.fn();
const success = { success: true, operation: "commit", output: "committed" };
const failure = { success: false, operation: "commit", output: "", error: "hook rejected" };
const rawTitle = "  Keep my title  ";
const rawBody = "  First line\n\nSecond line\n  ";
type DeferredCommit = {
  promise: Promise<typeof success>;
  resolve: (value: typeof success) => void;
  reject: (error: Error) => void;
};
const pendingRequests: DeferredCommit[] = [];
const happyWindow = window as unknown as HappyDOMWindow;

function deferred(): DeferredCommit {
  let resolve!: (value: typeof success) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<typeof success>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  const pending = { promise, resolve, reject };
  pendingRequests.push(pending);
  return pending;
}

function status(repo: string, staged = true): GitStatusEntry {
  return {
    repository_name: repo,
    status_state: "ready",
    files_complete: true,
    detail_state: "ready",
    branch: "feature",
    remote_branch: null,
    modified: ["file.ts"],
    added: [],
    deleted: [],
    renamed: [],
    untracked: [],
    ahead: 0,
    behind: 0,
    timestamp: "2026-10-08T13:00:00Z",
    files: { "file.ts": { path: "file.ts", status: "modified", staged } },
  };
}

function Openers() {
  const { openCommitDialog } = useVcsDialogs();
  return (
    <>
      <button data-testid="open-all" onClick={() => openCommitDialog()}>
        All
      </button>
      <button data-testid="open-root" onClick={() => openCommitDialog("")}>
        Root
      </button>
      <button data-testid="open-api" onClick={() => openCommitDialog("api")}>
        Api
      </button>
    </>
  );
}

function mountDialog(repositories = [""], sessionId: string | null = "session") {
  let store!: StoreApi<AppState>;
  function Initialize({ children }: { children: React.ReactNode }) {
    store = useAppStoreApi();
    useState(() => {
      store.setState({ environmentIdBySessionId: { session: "environment", other: "other-env" } });
      for (const environment of ["environment", "other-env"]) {
        for (const repo of repositories) store.getState().setGitStatus(environment, status(repo));
      }
      return true;
    });
    return children;
  }
  const tree = (sid: string | null) => (
    <StateProvider>
      <Initialize>
        <ToastProvider>
          <TooltipProvider>
            <VcsDialogsProvider sessionId={sid}>
              <Openers />
            </VcsDialogsProvider>
          </TooltipProvider>
        </ToastProvider>
      </Initialize>
    </StateProvider>
  );
  const view = render(tree(sessionId));
  return {
    store,
    unmount: view.unmount,
    changeSession: (sid: string | null) => view.rerender(tree(sid)),
  };
}

function open(scope = "all") {
  fireEvent.click(screen.getByTestId(`open-${scope}`));
}

function fill(title = rawTitle, body = rawBody, stageAll = true) {
  fireEvent.change(screen.getByTestId("commit-title-input"), { target: { value: title } });
  fireEvent.change(screen.getByTestId("commit-body-input"), { target: { value: body } });
  const checkbox = screen.getByRole("checkbox");
  if ((checkbox.getAttribute("aria-checked") === "true") !== stageAll) fireEvent.click(checkbox);
}

function expectDraft(title = rawTitle, body = rawBody, stageAll = true) {
  expect((screen.getByTestId("commit-title-input") as HTMLInputElement).value).toBe(title);
  expect((screen.getByTestId("commit-body-input") as HTMLTextAreaElement).value).toBe(body);
  expect(screen.getByRole("checkbox").getAttribute("aria-checked")).toBe(String(stageAll));
}

function submit() {
  fireEvent.click(screen.getByRole("button", { name: "Commit" }));
}

beforeEach(() => {
  commitRequest.mockReset().mockResolvedValue(success);
  const request = vi.fn(async (action: string, payload: unknown) => {
    if (action === "worktree.commit") return commitRequest(payload);
    if (action === "session.git.commits") return { commits: [] };
    if (action === "session.cumulative_diff") return { cumulative_diff: null };
    throw new Error(`Unexpected transport action ${action}`);
  });
  setWebSocketClient({ request } as unknown as WebSocketClient);
});

afterEach(async () => {
  await act(async () => {
    for (const pending of pendingRequests.splice(0)) pending.resolve(success);
  });
  cleanup();
  setWebSocketClient(null);
  happyWindow.happyDOM.setWindowSize({ width: 1280, height: 800 });
});

describe.each([
  ["desktop", 1280, 800],
  ["phone", 393, 851],
])("native commit draft on %s", (_surface, width, height) => {
  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.1, .2, .3, .4, .9
  it.each(["result", "rejection"])(
    "retains raw draft after %s and retries successfully",
    async (kind) => {
      happyWindow.happyDOM.setWindowSize({ width: Number(width), height: Number(height) });
      mountDialog(["", "api"]);
      open("api");
      fill();
      if (kind === "result") commitRequest.mockResolvedValueOnce(failure);
      else commitRequest.mockRejectedValueOnce(new Error("request rejected"));
      submit();
      await screen.findByText(kind === "result" ? "hook rejected" : "request rejected");
      expectDraft();
      expect(screen.getByRole("dialog").textContent).toContain("api");
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      open("api");
      expectDraft();
      expect(commitRequest).toHaveBeenCalledTimes(1);
      submit();
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(commitRequest).toHaveBeenLastCalledWith({
        session_id: "session",
        repo: "api",
        message: "Keep my title\n\nFirst line\n\nSecond line",
        stage_all: true,
        amend: false,
      });
      open("api");
      expectDraft("", "", false);
    },
  );
});

describe("native commit admission and settlement", () => {
  it("resets on ordinary success and preserves trimmed title-only payload", async () => {
    mountDialog();
    open();
    fill("  Title only  ", " \n ", false);
    submit();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(commitRequest).toHaveBeenCalledWith({
      session_id: "session",
      repo: "",
      message: "Title only",
      stage_all: false,
      amend: false,
    });
    expect(screen.getByText("committed")).toBeTruthy();
    open();
    expectDraft("", "", false);
  });

  it("admits no blank title and resets an ordinary dismissed unsubmitted draft", () => {
    mountDialog();
    open();
    fill(" \t ", rawBody);
    submit();
    expect(commitRequest).not.toHaveBeenCalled();
    expectDraft(" \t ");
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    open();
    expectDraft("", "", false);
  });

  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.5, .6, .9
  it.each([true, false])(
    "retains newer edits after pending success=%s, including viewport change",
    async (acknowledged) => {
      const pending = deferred();
      commitRequest.mockReturnValueOnce(pending.promise);
      mountDialog();
      open();
      fill();
      submit();
      expect(screen.getByRole("button", { name: /Committing/ }).hasAttribute("disabled")).toBe(
        true,
      );
      happyWindow.happyDOM.setWindowSize({ width: 393, height: 851 });
      fireEvent(window, new Event("resize"));
      fill("  New title  ", " new body\n ", false);
      expectDraft("  New title  ", " new body\n ", false);
      await act(async () => pending.resolve(acknowledged ? success : failure));
      expectDraft("  New title  ", " new body\n ", false);
      expect(commitRequest).toHaveBeenCalledTimes(1);
      expect(commitRequest.mock.calls[0][0].message).toBe(
        "Keep my title\n\nFirst line\n\nSecond line",
      );
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      open();
      expectDraft("  New title  ", " new body\n ", false);
    },
  );

  it.each([true, false])(
    "does not reopen after dismissing pending success=%s",
    async (acknowledged) => {
      const pending = deferred();
      commitRequest.mockReturnValueOnce(pending.promise);
      mountDialog();
      open();
      fill();
      submit();
      fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
      await act(async () => pending.resolve(acknowledged ? success : failure));
      expect(screen.queryByRole("dialog")).toBeNull();
      open();
      if (acknowledged) expectDraft("", "", false);
      else expectDraft();
    },
  );

  it("reopens pending draft without losing its busy guard", async () => {
    const pending = deferred();
    commitRequest.mockReturnValueOnce(pending.promise);
    mountDialog();
    open();
    fill();
    submit();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    open();
    expectDraft();
    fireEvent.click(screen.getByRole("button", { name: /Committing/ }));
    expect(commitRequest).toHaveBeenCalledTimes(1);
    await act(async () => pending.resolve(failure));
    expectDraft();
  });
});

describe("native commit scope and multi-repository behavior", () => {
  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.6, .7
  it("does not let an older scoped success clear a newly opened root draft", async () => {
    const pending = deferred();
    commitRequest.mockReturnValueOnce(pending.promise);
    mountDialog(["", "api"]);
    open("api");
    fill();
    submit();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    open("root");
    expectDraft("", "", false);
    fill("Root title", "Root body", false);
    await act(async () => pending.resolve(success));
    expectDraft("Root title", "Root body", false);
    submit();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(commitRequest).toHaveBeenLastCalledWith({
      session_id: "session",
      repo: "",
      message: "Root title\n\nRoot body",
      stage_all: false,
      amend: false,
    });
  });

  it.each(["session", "environment"])(
    "retires a pending owner across %s changes away and back",
    async (kind) => {
      const pending = deferred();
      commitRequest.mockReturnValueOnce(pending.promise);
      const view = mountDialog();
      open();
      fill();
      submit();
      if (kind === "session") {
        view.changeSession("other");
        view.changeSession("session");
      } else {
        act(() => view.store.setState({ environmentIdBySessionId: { session: "other-env" } }));
        act(() => view.store.setState({ environmentIdBySessionId: { session: "environment" } }));
      }
      expect(screen.queryByRole("dialog")).toBeNull();
      open();
      expectDraft("", "", false);
      fill("Replacement", "", false);
      await act(async () => pending.resolve(success));
      expectDraft("Replacement", "", false);
    },
  );

  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.8
  it("retains an aggregate partial failure and retries only current staged repositories", async () => {
    const view = mountDialog(["api", "web"]);
    open();
    fill(rawTitle, rawBody, false);
    commitRequest.mockImplementation(async ({ repo }) => (repo === "api" ? failure : success));
    submit();
    await screen.findByText("hook rejected");
    expectDraft(rawTitle, rawBody, false);
    expect(commitRequest.mock.calls.map(([payload]) => payload.repo)).toEqual(["api", "web"]);
    const clean = status("web");
    clean.files = {};
    clean.modified = [];
    clean.timestamp = "2026-10-08T13:00:01Z";
    act(() => {
      view.store.getState().setGitStatus("environment", clean);
    });
    commitRequest.mockResolvedValue(success);
    submit();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(commitRequest.mock.calls.map(([payload]) => payload.repo)).toEqual([
      "api",
      "web",
      "api",
    ]);
  });

  it("does not submit without a session", () => {
    mountDialog([""], null);
    open();
    fill();
    submit();
    expect(commitRequest).not.toHaveBeenCalled();
    expectDraft();
  });
});
