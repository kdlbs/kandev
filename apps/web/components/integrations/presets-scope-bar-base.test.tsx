import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationScopeBar, type IntegrationScopeBarProps } from "./presets-scope-bar-base";

let lastMenuItemSelectEvent: Event | null = null;

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => (
    <div role="menu">{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    disabled,
    onSelect,
    ...props
  }: React.HTMLAttributes<HTMLDivElement> & {
    disabled?: boolean;
    onSelect?: (event: Event) => void;
  }) => {
    const select = () => {
      if (disabled) return;
      const event = new Event("select", { cancelable: true });
      lastMenuItemSelectEvent = event;
      onSelect?.(event);
    };
    return (
      <div
        {...props}
        role="menuitem"
        aria-disabled={disabled}
        onClick={select}
        onPointerUp={select}
      >
        {children}
      </div>
    );
  },
  DropdownMenuCheckboxItem: ({
    children,
    checked,
    disabled,
    onCheckedChange,
    onSelect,
    ...props
  }: React.HTMLAttributes<HTMLDivElement> & {
    checked?: boolean;
    disabled?: boolean;
    onCheckedChange?: (checked: boolean) => void;
    onSelect?: (event: Event) => void;
  }) => (
    <div
      {...props}
      role="menuitemcheckbox"
      aria-checked={checked}
      aria-disabled={disabled}
      onClick={() => {
        if (disabled) return;
        onSelect?.(new Event("select", { cancelable: true }));
        onCheckedChange?.(!checked);
      }}
    >
      {children}
    </div>
  ),
  DropdownMenuSeparator: () => <hr />,
}));

afterEach(() => {
  cleanup();
  lastMenuItemSelectEvent = null;
});

type Kind = "pr" | "issue";
type Props = IntegrationScopeBarProps<Kind>;
const ARIA_DISABLED_ATTRIBUTE = "aria-disabled";
const FUTURE_DELETE_LABEL = "Delete Future default saved query";

function renderBar(overrides: Partial<Props> = {}) {
  const onSelect = vi.fn();
  const onToggleSavedDefault = vi.fn();
  const onDeleteSaved = vi.fn();
  const props: Props = {
    testId: "scope-bar",
    savedMenuTestId: "saved-menu",
    kinds: [
      { value: "pr", label: "Pull requests" },
      { value: "issue", label: "Issues" },
    ],
    selected: { kind: "pr", source: "preset", id: "review" },
    onSelect,
    presetsByKind: () => [
      {
        value: "review",
        label: "Review requested",
        icon: IconInbox,
        group: "inbox",
      },
    ],
    savedPresets: [
      { id: "saved-a", kind: "pr", label: "Current default", isDefault: true },
      { id: "saved-b", kind: "pr", label: "Future default", isDefault: false },
    ],
    onDeleteSaved,
    canSaveCurrent: false,
    onSaveCurrent: vi.fn(),
    onToggleSavedDefault,
    ...overrides,
  };
  return {
    ...render(<IntegrationScopeBar {...props} />),
    onSelect,
    onToggleSavedDefault,
    onDeleteSaved,
  };
}

function expectActiveKindReselectionNoop() {
  const { onSelect } = renderBar();

  fireEvent.click(screen.getByRole("button", { name: "Pull requests" }));

  expect(onSelect).not.toHaveBeenCalled();
}

describe("IntegrationScopeBar saved defaults", () => {
  it("ignores reselecting the active kind", expectActiveKindReselectionNoop);

  it("delegates kind changes to the explicit callback when provided", () => {
    const onKindChange = vi.fn();
    const { onSelect } = renderBar({ onKindChange });

    fireEvent.click(screen.getByRole("button", { name: "Issues" }));

    expect(onKindChange).toHaveBeenCalledWith("issue");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("renders set and clear markers without selecting the saved query", () => {
    const { onSelect, onToggleSavedDefault } = renderBar();
    const clear = screen.getByRole("menuitemcheckbox", {
      name: "Clear Current default as default view",
    });
    const set = screen.getByRole("menuitemcheckbox", {
      name: "Set Future default as default view",
    });

    expect(clear.getAttribute("aria-checked")).toBe("true");
    expect(set.getAttribute("aria-checked")).toBe("false");
    expect(set.parentElement?.getAttribute("role")).toBe("none");
    fireEvent.click(set);

    expect(onToggleSavedDefault).toHaveBeenCalledWith("saved-b");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("disables every default action while a mutation is pending", () => {
    renderBar({ defaultMutationPendingId: "saved-b" });
    const currentDefault = screen.getByRole("menuitemcheckbox", {
      name: "Updating default view… Clear Current default as default view",
    });
    const currentDelete = screen.getByRole("menuitem", {
      name: "Delete Current default saved query",
    });

    expect(currentDefault.getAttribute(ARIA_DISABLED_ATTRIBUTE)).toBe("true");
    expect(currentDefault.getAttribute("aria-busy")).toBeNull();
    expect(currentDefault.querySelector("svg")?.getAttribute("class")).not.toContain(
      "animate-pulse",
    );
    const futureDefault = screen.getByRole("menuitemcheckbox", {
      name: "Updating default view… Set Future default as default view",
    });
    expect(futureDefault.getAttribute(ARIA_DISABLED_ATTRIBUTE)).toBe("true");
    expect(futureDefault.querySelector("svg")?.getAttribute("class")).toContain("fill-amber-500");
    expect(futureDefault.querySelector("svg")?.getAttribute("class")).toContain("opacity-60");
    expect(futureDefault.querySelector("svg")?.getAttribute("class")).toContain("animate-pulse");
    expect(currentDelete.getAttribute(ARIA_DISABLED_ATTRIBUTE)).toBe("true");
    expect(currentDelete.className).toContain("group-hover/saved:data-[disabled]:opacity-50");
    expect(
      screen
        .getByRole("menuitem", {
          name: FUTURE_DELETE_LABEL,
        })
        .getAttribute(ARIA_DISABLED_ATTRIBUTE),
    ).toBe("true");
  });

  it("renders no default actions when an integration omits the optional contract", () => {
    renderBar({ onToggleSavedDefault: undefined });

    expect(screen.queryByRole("menuitemcheckbox", { name: /as default view/ })).toBeNull();
  });

  it("names each delete action for its saved query", () => {
    renderBar();

    expect(
      screen
        .getByRole("menuitem", { name: "Delete Current default saved query" })
        .getAttribute("title"),
    ).toBe("Delete Current default saved query");
    expect(screen.getByRole("menuitem", { name: FUTURE_DELETE_LABEL }).getAttribute("title")).toBe(
      FUTURE_DELETE_LABEL,
    );
  });

  it("prevents the menu from closing when a saved query is deleted", () => {
    const { onDeleteSaved } = renderBar();

    fireEvent.click(screen.getByRole("menuitem", { name: FUTURE_DELETE_LABEL }));

    expect(onDeleteSaved).toHaveBeenCalledWith("saved-b");
    expect(lastMenuItemSelectEvent?.defaultPrevented).toBe(true);
  });
});
