import { afterEach, describe, expect, it } from "vitest";
import { clearLspLeaseHint, getLspLeaseHint, saveLspLeaseHint } from "./lsp-client-storage";

describe("LSP lease hints", () => {
  afterEach(() => {
    sessionStorage.clear();
  });

  it("stores hints per tab, task, and language", () => {
    saveLspLeaseHint("task-a", "go", "lease-a");
    saveLspLeaseHint("task-a", "typescript", "lease-b");
    saveLspLeaseHint("task-b", "go", "lease-c");

    expect(getLspLeaseHint("task-a", "go")).toBe("lease-a");
    expect(getLspLeaseHint("task-a", "typescript")).toBe("lease-b");
    expect(getLspLeaseHint("task-b", "go")).toBe("lease-c");
  });

  it("clears only the selected lease hint", () => {
    saveLspLeaseHint("task-a", "go", "lease-a");
    saveLspLeaseHint("task-a", "typescript", "lease-b");

    clearLspLeaseHint("task-a", "go");

    expect(getLspLeaseHint("task-a", "go")).toBeNull();
    expect(getLspLeaseHint("task-a", "typescript")).toBe("lease-b");
  });
});
