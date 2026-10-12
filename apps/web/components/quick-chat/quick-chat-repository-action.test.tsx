import { fireEvent, render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { QuickChatRepositoryAction } from "./quick-chat-repository-action";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Repository } from "@/lib/types/http";

vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => false }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));
afterEach(cleanup);
const repositories = [
  { id: "repo-a", name: "Alpha", local_path: "/alpha" },
  { id: "repo-b", name: "Beta", local_path: "/beta" },
] as Repository[];

describe("Quick Chat repository action", () => {
  it("opens a picker without adding a row and adds only the selected repository", () => {
    const onSelect = vi.fn();
    render(
      <TooltipProvider>
        <QuickChatRepositoryAction
          repositories={repositories}
          selectedIds={[]}
          disabled={false}
          onSelect={onSelect}
        />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add repository" }));
    expect(onSelect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("option", { name: /Alpha/ }));
    expect(onSelect).toHaveBeenCalledExactlyOnceWith("repo-a");
  });

  it("excludes repositories already represented by chips", () => {
    render(
      <TooltipProvider>
        <QuickChatRepositoryAction
          repositories={repositories}
          selectedIds={["repo-a"]}
          disabled={false}
          onSelect={vi.fn()}
        />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Add repository" }));
    expect(screen.queryByRole("option", { name: /Alpha/ })).toBeNull();
    expect(screen.getByRole("option", { name: /Beta/ })).toBeTruthy();
  });
});
