import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Branch } from "@/lib/types/http";
import { BranchPickerList } from "./branch-picker-list";

afterEach(() => cleanup());

describe("BranchPickerList", () => {
  it("puts the current branch first and keeps its selected surface", () => {
    const branches: Branch[] = [
      { name: "feature/local", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
      { name: "develop", type: "remote", remote: "origin" },
    ];

    render(
      <BranchPickerList
        branches={branches}
        isLoadingBranches={false}
        currentBase="main"
        onSelect={() => {}}
      />,
    );

    const options = screen.getAllByRole("option");
    expect(options.map((option) => option.textContent?.trim())).toEqual([
      "main",
      "feature/local",
      "develop",
    ]);
    expect(options[0].getAttribute("aria-selected")).toBe("true");
    expect(options[0].className).toContain("bg-card");
    expect(options[0].className).toContain("border-primary/50");
  });

  it("does not inject the current branch into filtered results", () => {
    render(
      <BranchPickerList
        branches={[
          { name: "main", type: "remote", remote: "origin" },
          { name: "develop", type: "remote", remote: "origin" },
        ]}
        isLoadingBranches={false}
        currentBase="main"
        onSelect={() => {}}
      />,
    );

    const filter = screen.getByTestId("base-branch-picker-filter");
    fireEvent.change(filter, { target: { value: "develop" } });

    expect(screen.getAllByRole("option").map((option) => option.textContent?.trim())).toEqual([
      "develop",
    ]);
  });

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
