import { afterEach, describe, it, expect, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import type { ReactNode } from "react";
import type { Branch } from "@/lib/types/http";

const TOOLTIP_ROOT_TEST_ID = "tooltip-root";

vi.mock("@kandev/ui/tooltip", async () => {
  const React = await import("react");
  const TooltipContext = React.createContext<((open: boolean) => void) | undefined>(undefined);
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
      return (
        <TooltipContext.Provider value={onOpenChange}>
          <div data-testid={TOOLTIP_ROOT_TEST_ID} data-open={String(open)}>
            {children}
          </div>
        </TooltipContext.Provider>
      );
    },
    TooltipTrigger: ({
      children,
      onPointerEnter: triggerPointerEnter,
      onPointerLeave: triggerPointerLeave,
      onFocus: triggerFocus,
      onBlur: triggerBlur,
      ...triggerProps
    }: {
      children: ReactNode;
      onPointerEnter?: React.PointerEventHandler;
      onPointerLeave?: React.PointerEventHandler;
      onFocus?: React.FocusEventHandler;
      onBlur?: React.FocusEventHandler;
      [key: string]: unknown;
    }) => {
      const onOpenChange = React.useContext(TooltipContext);
      const child = React.Children.only(children) as React.ReactElement<{
        onPointerEnter?: React.PointerEventHandler;
        onPointerLeave?: React.PointerEventHandler;
        onFocus?: React.FocusEventHandler;
        onBlur?: React.FocusEventHandler;
      }>;
      return React.cloneElement(child, {
        ...triggerProps,
        onPointerEnter: (event) => {
          triggerPointerEnter?.(event);
          child.props.onPointerEnter?.(event);
          onOpenChange?.(true);
        },
        onPointerLeave: (event) => {
          triggerPointerLeave?.(event);
          child.props.onPointerLeave?.(event);
          onOpenChange?.(false);
        },
        onFocus: (event) => {
          triggerFocus?.(event);
          child.props.onFocus?.(event);
          onOpenChange?.(true);
        },
        onBlur: (event) => {
          triggerBlur?.(event);
          child.props.onBlur?.(event);
          onOpenChange?.(false);
        },
      });
    },
    TooltipContent: ({ children, className }: { children: ReactNode; className?: string }) => (
      <div data-testid="tooltip-content" className={className}>
        {children}
      </div>
    ),
  };
});

import {
  sortBranches,
  branchToOption,
  computeBranchPlaceholder,
  Pill,
  type PillOption,
} from "./task-create-dialog-pill";

const CREATE_REPOSITORY = "Create new repository";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function localBranch(name: string): Branch {
  return { name, type: "local" } as Branch;
}

function remoteBranch(name: string, remote = "origin"): Branch {
  return { name, type: "remote", remote } as Branch;
}

const TOOLTIP_PILL_VALUE = "kandev";
const REPOSITORY_TOOLTIP = "Repository · ~/kandev";

function renderTooltipPill({
  value = TOOLTIP_PILL_VALUE,
  options = [],
  tooltip = REPOSITORY_TOOLTIP,
}: {
  value?: string;
  options?: PillOption[];
  tooltip?: string;
} = {}) {
  render(
    <Pill
      icon={<span aria-hidden="true" />}
      value={value}
      placeholder="repository"
      options={options}
      onSelect={vi.fn()}
      searchPlaceholder="Search repositories..."
      emptyMessage="No repositories"
      tooltip={tooltip}
    />,
  );
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

    renderTooltipPill();

    await waitFor(() => {
      expect(screen.getByTestId(TOOLTIP_ROOT_TEST_ID).getAttribute("data-open")).toBe("false");
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

  it("allows the first later mouse hover when the pointer left for the picker", async () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    renderTooltipPill({ options: [{ value: "repo-1", label: "repo-one" }] });

    const trigger = screen.getByRole("button", { name: new RegExp(TOOLTIP_PILL_VALUE, "i") });
    fireEvent.click(trigger);
    fireEvent.pointerLeave(trigger);
    fireEvent.click(await screen.findByRole("option", { name: "repo-one" }));
    fireEvent.pointerEnter(trigger);

    expect(screen.getByTestId(TOOLTIP_ROOT_TEST_ID).getAttribute("data-open")).toBe("true");
  });

  it("keeps touch selection tooltip suppression through focus restoration", async () => {
    const frameCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      frameCallbacks.push(callback);
      return frameCallbacks.length;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    renderTooltipPill({ options: [{ value: "repo-1", label: "repo-one" }] });
    frameCallbacks.shift()?.(0);

    const trigger = screen.getByRole("button", { name: new RegExp(TOOLTIP_PILL_VALUE, "i") });
    fireEvent.pointerEnter(trigger, { pointerType: "touch" });
    fireEvent.click(trigger);
    const option = await screen.findByRole("option", { name: "repo-one" });
    fireEvent.pointerDown(option, { pointerType: "touch" });
    fireEvent.click(option);
    fireEvent.pointerLeave(trigger, { pointerType: "touch" });
    frameCallbacks.shift()?.(0);
    frameCallbacks.shift()?.(0);
    fireEvent.blur(trigger);
    fireEvent.focus(trigger);

    expect(screen.getByTestId(TOOLTIP_ROOT_TEST_ID).getAttribute("data-open")).toBe("false");

    fireEvent.pointerEnter(trigger, { pointerType: "mouse" });

    expect(screen.getByTestId(TOOLTIP_ROOT_TEST_ID).getAttribute("data-open")).toBe("true");
  });

  it("wraps long repository paths inside a viewport-safe tooltip", () => {
    renderTooltipPill({
      value: "repository",
      tooltip: `Repository · C:\\${"unbroken-path-segment".repeat(20)}`,
    });

    const classes = screen.getByTestId("tooltip-content").classList;
    expect(classes.contains("max-w-[calc(100vw-2rem)]")).toBe(true);
    expect(classes.contains("break-all")).toBe(true);
  });

  it("releases picker tooltip suppression when the keyboard leaves the trigger", async () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 1;
    });
    vi.stubGlobal("cancelAnimationFrame", vi.fn());

    renderTooltipPill({ options: [{ value: "repo-1", label: "repo-one" }] });

    const trigger = screen.getByRole("button", { name: new RegExp(TOOLTIP_PILL_VALUE, "i") });
    fireEvent.click(trigger);
    fireEvent.click(await screen.findByRole("option", { name: "repo-one" }));
    fireEvent.blur(trigger);
    fireEvent.focus(trigger);

    expect(screen.getByTestId(TOOLTIP_ROOT_TEST_ID).getAttribute("data-open")).toBe("true");
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

  it("activates an optional toolbar action with a pointer", () => {
    const onAction = vi.fn();
    render(
      <Pill
        icon={<span aria-hidden="true" />}
        value=""
        placeholder="repository"
        options={[]}
        onSelect={vi.fn()}
        searchPlaceholder="Search repositories..."
        emptyMessage="No repositories"
        action={{ label: CREATE_REPOSITORY, onSelect: onAction }}
      />,
    );

    fireEvent.click(screen.getByText("repository"));
    fireEvent.click(screen.getByRole("button", { name: CREATE_REPOSITORY }));

    expect(onAction).toHaveBeenCalledOnce();
  });

  it("renders an optional toolbar action outside the option list", () => {
    const onAction = vi.fn();
    render(
      <Pill
        icon={<span aria-hidden="true" />}
        value=""
        placeholder="repository"
        options={[]}
        onSelect={vi.fn()}
        searchPlaceholder="Search repositories..."
        emptyMessage="No repositories"
        action={{ label: CREATE_REPOSITORY, onSelect: onAction }}
      />,
    );

    fireEvent.click(screen.getByText("repository"));
    expect(screen.getByRole("button", { name: CREATE_REPOSITORY })).toBeTruthy();
    expect(screen.queryByRole("option", { name: CREATE_REPOSITORY })).toBeNull();
    expect(onAction).not.toHaveBeenCalled();
  });
});
