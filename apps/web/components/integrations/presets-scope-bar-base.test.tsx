import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { IconInbox } from "@tabler/icons-react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { IntegrationScopeBar, type IntegrationScopeBarProps } from "./presets-scope-bar-base";

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
    onSelect?: () => void;
  }) => (
    <div
      {...props}
      role="menuitem"
      aria-disabled={disabled}
      onClick={() => !disabled && onSelect?.()}
      onPointerUp={() => !disabled && onSelect?.()}
    >
      {children}
    </div>
  ),
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

afterEach(cleanup);

type Kind = "pr" | "issue";
type Props = IntegrationScopeBarProps<Kind>;
const ARIA_DISABLED_ATTRIBUTE = "aria-disabled";

function renderBar(overrides: Partial<Props> = {}) {
  const onSelect = vi.fn();
  const onToggleSavedDefault = vi.fn();
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
    onDeleteSaved: vi.fn(),
    canSaveCurrent: false,
    onSaveCurrent: vi.fn(),
    onToggleSavedDefault,
    ...overrides,
  };
  return {
    ...render(<IntegrationScopeBar {...props} />),
    onSelect,
    onToggleSavedDefault,
  };
}

describe("IntegrationScopeBar saved defaults", () => {
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
    renderBar({ defaultMutationPending: true });
    const currentDefault = screen.getByRole("menuitemcheckbox", {
      name: "Updating default view… Clear Current default as default view",
    });
    const currentDelete = screen.getByRole("menuitem", {
      name: "Delete Current default saved query",
    });

    expect(currentDefault.getAttribute(ARIA_DISABLED_ATTRIBUTE)).toBe("true");
    expect(currentDefault.getAttribute("aria-busy")).toBeNull();
    expect(currentDefault.querySelector("svg")?.getAttribute("class")).toContain("animate-pulse");
    expect(
      screen
        .getByRole("menuitemcheckbox", {
          name: "Updating default view… Set Future default as default view",
        })
        .getAttribute(ARIA_DISABLED_ATTRIBUTE),
    ).toBe("true");
    expect(currentDelete.getAttribute(ARIA_DISABLED_ATTRIBUTE)).toBe("true");
    expect(currentDelete.className).toContain("group-hover/saved:data-[disabled]:opacity-50");
    expect(
      screen
        .getByRole("menuitem", {
          name: "Delete Future default saved query",
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
    expect(
      screen
        .getByRole("menuitem", { name: "Delete Future default saved query" })
        .getAttribute("title"),
    ).toBe("Delete Future default saved query");
  });
});
