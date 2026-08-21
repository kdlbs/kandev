import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Branch } from "@/lib/types/http";
import { BranchPickerList } from "./branch-picker-list";

afterEach(() => cleanup());

describe("BranchPickerList", () => {
  it("can limit recovery choices to remote branches", () => {
    const branches: Branch[] = [
      { name: "feature/local-only", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
    ];

    render(
      <BranchPickerList
        branches={branches}
        isLoadingBranches={false}
        currentBase="main"
        onSelect={() => {}}
        remoteOnly
        testIdPrefix="recovery-branch"
      />,
    );

    expect(screen.queryByTestId("recovery-branch-option-feature/local-only")).toBeNull();
    expect(screen.getByTestId("recovery-branch-option-main")).toBeTruthy();
  });
});
