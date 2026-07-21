import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import type { Branch } from "@/lib/types/http";

vi.mock("@kandev/ui/tooltip", async () => {
  const React = await import("react");
  return {
    Tooltip: ({
      open,
      onOpenChange,
      children,
    }: {
      open?: boolean;
      onOpenChange?: (open: boolean) => void;
      children: ReactNode;
    }) => {
      React.useEffect(() => {
        onOpenChange?.(true);
      }, [onOpenChange]);
      return (
        <div data-testid="tooltip-root" data-open={String(open)}>
          {children}
        </div>
      );
    },
    TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
    TooltipContent: ({ children }: { children: ReactNode }) => (
      <div data-testid="tooltip-content">{children}</div>
    ),
  };
});

import {
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
  Pill,
} from "./task-create-dialog-pill";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function localBranch(name: string): Branch {
  return { name, type: "local" } as Branch;
}

function remoteBranch(name: string, remote = "origin"): Branch {
  return { name, type: "remote", remote } as Branch;
}

describe("sortBranches", () => {
  it("lifts main before master before develop before other branches", () => {
    const sorted = sortBranches([
      localBranch("feature/a"),
      localBranch("develop"),
      localBranch("master"),
      localBranch("main"),
    ]);
    expect(sorted.map((b) => b.name)).toEqual(["main", "master", "develop", "feature/a"]);
  });

  it("puts local before remote when both have the same preferred name", () => {
    const sorted = sortBranches([remoteBranch("main"), localBranch("main")]);
    expect(sorted.map((b) => b.type)).toEqual(["local", "remote"]);
  });

  it("leaves non-preferred branches in their original relative order", () => {
    const sorted = sortBranches([
      localBranch("feature/zeta"),
      localBranch("feature/alpha"),
      localBranch("feature/middle"),
    ]);
    expect(sorted.map((b) => b.name)).toEqual(["feature/zeta", "feature/alpha", "feature/middle"]);
  });

  it("does not mutate the input array", () => {
    const input = [localBranch("feature/a"), localBranch("main")];
    const snapshot = input.map((b) => b.name);
    sortBranches(input);
    expect(input.map((b) => b.name)).toEqual(snapshot);
  });
});

describe("branchToOption keywords", () => {
  function keywords(b: Branch): string[] {
    return branchToOption(b).keywords ?? [];
  }

  it("includes the leaf segment for slash-prefixed branches", () => {
    expect(keywords(localBranch("feat/scope/thing"))).toContain("thing");
  });

  it("splits on slashes, dots, underscores, and hyphens", () => {
    const kw = keywords(localBranch("feat/scope.thing_with-dash"));
    expect(kw).toEqual(expect.arrayContaining(["feat", "scope", "thing", "with", "dash"]));
  });

  it("includes the remote name when present", () => {
    expect(keywords(remoteBranch("main", "upstream"))).toContain("upstream");
  });

  it("dedupes repeated segments", () => {
    const kw = keywords(localBranch("foo/foo"));
    expect(kw.filter((k) => k === "foo")).toHaveLength(1);
  });
});

describe("computeBranchPlaceholder", () => {
  it("returns 'branch' when no repo is selected", () => {
    expect(computeBranchPlaceholder(false, false, 0)).toBe("branch");
  });

  it("returns 'loading…' while branches are loading", () => {
    expect(computeBranchPlaceholder(true, true, 0)).toBe("loading…");
  });

  it("returns 'no branches' when loaded but the list is empty", () => {
    expect(computeBranchPlaceholder(true, false, 0)).toBe("no branches");
  });

  it("returns 'branch' as the default with options available", () => {
    expect(computeBranchPlaceholder(true, false, 3)).toBe("branch");
  });
});

describe("Pill tooltip", () => {
  it("ignores tooltip open requests during the initial mount frame", async () => {
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn(() => 1),
    );
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    render(
      <Pill
        icon={<span aria-hidden="true" />}
        value="kandev"
        placeholder="repository"
        options={[]}
        onSelect={vi.fn()}
        searchPlaceholder="Search repositories..."
        emptyMessage="No repositories"
        tooltip="Repository · ~/kandev"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("tooltip-root").getAttribute("data-open")).toBe("false");
    });
  });

  it("makes the disabled tooltip wrapper keyboard focusable", () => {
    render(
      <Pill
        icon={<span aria-hidden="true" />}
        value=""
        placeholder="branch"
        options={[]}
        onSelect={vi.fn()}
        searchPlaceholder="Search branches..."
        emptyMessage="No branches"
        disabled
        disabledReason="Select a repository first"
      />,
    );

    const tooltipTrigger = screen.getByLabelText("Select a repository first");
    expect(tooltipTrigger.getAttribute("tabindex")).toBe("0");
    expect(tooltipTrigger.querySelector("[aria-hidden='true'] button")).not.toBeNull();
  });
});

describe("Pill popover", () => {
  it("portals selector content outside the trigger container", () => {
    render(
      <div data-testid="clipping-host" className="overflow-hidden">
        <Pill
          icon={<span aria-hidden="true" />}
          value="kandev"
          placeholder="repository"
          options={[{ value: "repo-1", label: "repo-one" }]}
          onSelect={vi.fn()}
          searchPlaceholder="Search repositories..."
          emptyMessage="No repositories"
        />
      </div>,
    );

    fireEvent.click(within(screen.getByTestId("clipping-host")).getByText("kandev"));

    const content = document.body.querySelector('[data-slot="popover-content"]');
    expect(content).not.toBeNull();
    expect(screen.getByTestId("clipping-host").contains(content)).toBe(false);
  });
});
