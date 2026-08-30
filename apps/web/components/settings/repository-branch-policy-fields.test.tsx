import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Branch } from "@/lib/types/http";
import { BranchPolicyBranchPicker } from "./repository-branch-policy-fields";

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));

afterEach(cleanup);

describe("BranchPolicyBranchPicker", () => {
  it("filters all local and remote branches and refreshes from the picker", () => {
    const onChange = vi.fn();
    const onRefresh = vi.fn();
    const branches: Branch[] = [
      { name: "main", type: "local" },
      { name: "main", type: "remote", remote: "origin" },
      { name: "release/candidate", type: "remote", remote: "upstream" },
    ];

    render(
      <TooltipProvider>
        <BranchPolicyBranchPicker
          label="Base branch"
          value="main"
          onChange={onChange}
          branches={branches}
          onRefresh={onRefresh}
          loading={false}
          testId="policy-base-picker"
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("combobox", { name: "Base branch" }));
    expect(screen.getByPlaceholderText("Search branches...")).toBeTruthy();
    expect(screen.getByRole("option", { name: /^main local/ })).toBeTruthy();
    expect(screen.getByRole("option", { name: /^origin\/main origin/ })).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText("Search branches..."), {
      target: { value: "candidate" },
    });
    const listbox = screen.getByRole("listbox");
    expect(
      within(listbox).getByRole("option", { name: /^upstream\/release\/candidate upstream/ }),
    ).toBeTruthy();
    expect(within(listbox).queryByRole("option", { name: /^main local/ })).toBeNull();

    fireEvent.click(screen.getByTestId("branch-refresh-button"));
    expect(onRefresh).toHaveBeenCalledOnce();
  });
});
