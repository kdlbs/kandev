import { act, cleanup, renderHook } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { useCommitDialogState } from "./use-commit-dialog-state";

afterEach(cleanup);

function mount(sessionId: string | null = "session") {
  return renderHook(({ session, environment }) => useCommitDialogState(session, environment), {
    initialProps: { session: sessionId, environment: "env" },
    wrapper: ({ children }: { children: ReactNode }) => <StrictMode>{children}</StrictMode>,
  });
}

describe("commit draft ownership and admission", () => {
  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.4, .5
  it("rejects same-tick duplicate submissions and retains edits even when reverted", () => {
    const { result } = mount();
    act(() => result.current.openDialog("api"));
    act(() => result.current.setMessage(" title "));
    let attempt!: NonNullable<ReturnType<typeof result.current.begin>>;
    act(() => {
      attempt = result.current.begin(false)!;
      expect(result.current.begin(false)).toBeNull();
    });
    expect(attempt.message).toBe(" title ");
    act(() => {
      result.current.setMessage("edited");
      result.current.setMessage(" title ");
    });
    act(() => result.current.settle(attempt, true));
    expect(result.current.open).toBe(true);
    expect(result.current.message).toBe(" title ");
    expect(result.current.pending).toBe(false);
  });

  it("refuses blank, busy and missing-session admission without consuming the draft", () => {
    const { result, rerender } = mount();
    act(() => result.current.openDialog());
    act(() => result.current.setMessage(" \t "));
    expect(result.current.begin(false)).toBeNull();
    act(() => result.current.setMessage("Valid"));
    expect(result.current.begin(true)).toBeNull();
    rerender({ session: null, environment: "env" });
    act(() => {
      result.current.openDialog();
    });
    act(() => result.current.setMessage("Valid"));
    expect(result.current.begin(false)).toBeNull();
    expect(result.current.message).toBe("Valid");
    expect(result.current.pending).toBe(false);
  });

  it.each(["title", "body", "stageAll"])(
    "preserves a pending %s edit after older success",
    (field) => {
      const { result } = mount();
      act(() => result.current.openDialog());
      act(() => result.current.setMessage("Title"));
      let attempt!: NonNullable<ReturnType<typeof result.current.begin>>;
      act(() => {
        attempt = result.current.begin(false)!;
      });
      act(() => {
        if (field === "title") result.current.setMessage("New title");
        else if (field === "body") result.current.setBody("New body");
        else result.current.setStageAll(true);
      });
      act(() => result.current.settle(attempt, true));
      expect(result.current.open).toBe(true);
      expect(result.current.message).toBe(field === "title" ? "New title" : "Title");
      expect(result.current.body).toBe(field === "body" ? "New body" : "");
      expect(result.current.stageAll).toBe(field === "stageAll");
    },
  );
});

describe("commit repository and lifetime ownership", () => {
  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.6
  it("keeps newer pending ownership when an older repository completion arrives", () => {
    const { result } = mount();
    act(() => result.current.openDialog("api"));
    act(() => result.current.setMessage("Api"));
    let old!: NonNullable<ReturnType<typeof result.current.begin>>;
    act(() => {
      old = result.current.begin(false)!;
    });
    act(() => result.current.openDialog(""));
    act(() => result.current.setMessage("Root"));
    let current!: NonNullable<ReturnType<typeof result.current.begin>>;
    act(() => {
      current = result.current.begin(false)!;
    });
    act(() => result.current.settle(old, true));
    expect(result.current.message).toBe("Root");
    expect(result.current.pending).toBe(true);
    expect(result.current.begin(false)).toBeNull();
    act(() => result.current.settle(current, false));
    expect(result.current.pending).toBe(false);
    expect(result.current.repo).toBe("");
  });

  it("distinguishes omitted, root and named scopes and normalizes event arguments", () => {
    const { result } = mount();
    act(() => result.current.openDialog(""));
    act(() => result.current.setMessage("Root"));
    let attempt!: NonNullable<ReturnType<typeof result.current.begin>>;
    act(() => {
      attempt = result.current.begin(false)!;
    });
    act(() => result.current.settle(attempt, false));
    act(() => result.current.openDialog(""));
    expect(result.current.message).toBe("Root");
    act(() => result.current.openDialog());
    expect(result.current.repo).toBeUndefined();
    expect(result.current.message).toBe("");
    act(() => result.current.openDialog("api"));
    expect(result.current.repo).toBe("api");
    act(() => result.current.openDialog({ type: "click" } as unknown as string));
    expect(result.current.repo).toBeUndefined();
  });

  // @covers AC-WORKSPACES-COMMIT-DRAFT-RETRY-001.7
  it.each(["session", "environment"])(
    "retires captured callbacks after %s changes away and back",
    (key) => {
      const { result, rerender } = mount();
      act(() => result.current.openDialog());
      act(() => result.current.setMessage("Old"));
      const old = result.current;
      rerender({ session: "session", environment: "env", [key]: "other" });
      rerender({ session: "session", environment: "env" });
      act(() => result.current.openDialog());
      act(() => result.current.setMessage("New"));
      act(() => {
        old.setMessage("Stale");
      });
      expect(old.begin(false)).toBeNull();
      expect(result.current.message).toBe("New");
    },
  );

  it("retires admission and settlement after unmount", () => {
    const { result, unmount } = mount();
    act(() => result.current.openDialog());
    act(() => result.current.setMessage("Old"));
    let attempt!: NonNullable<ReturnType<typeof result.current.begin>>;
    act(() => {
      attempt = result.current.begin(false)!;
    });
    const old = result.current;
    unmount();
    expect(old.begin(false)).toBeNull();
    act(() => old.settle(attempt, true));
    expect(result.current.message).toBe("Old");
  });
});
