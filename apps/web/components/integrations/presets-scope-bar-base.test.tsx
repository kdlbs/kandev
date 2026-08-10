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
  DropdownMenuSeparator: () => <hr />,
}));

afterEach(cleanup);

type Kind = "pr" | "issue";
type Props = IntegrationScopeBarProps<Kind>;

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
    const clear = screen.getByRole("button", {
      name: "Clear Current default as default view",
    });
    const set = screen.getByRole("button", {
      name: "Set Future default as default view",
    });

    expect(clear.getAttribute("aria-pressed")).toBe("true");
    expect(set.getAttribute("aria-pressed")).toBe("false");
    fireEvent.pointerDown(set);
    fireEvent.pointerUp(set);
    fireEvent.click(set);

    expect(onToggleSavedDefault).toHaveBeenCalledWith("saved-b");
    expect(onSelect).not.toHaveBeenCalled();
  });

  it("disables every default action while a mutation is pending", () => {
    renderBar({ defaultMutationPending: true });
    const currentDelete = screen.getByRole("button", {
      name: "Delete Current default saved query",
    }) as HTMLButtonElement;

    expect(
      (
        screen.getByRole("button", {
          name: "Clear Current default as default view",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(
      (
        screen.getByRole("button", {
          name: "Set Future default as default view",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(currentDelete.disabled).toBe(true);
    expect(currentDelete.className).toContain("group-hover/saved:disabled:opacity-50");
    expect(
      (
        screen.getByRole("button", {
          name: "Delete Future default saved query",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it("renders no default actions when an integration omits the optional contract", () => {
    renderBar({ onToggleSavedDefault: undefined });

    expect(screen.queryByRole("button", { name: /as default view/ })).toBeNull();
  });

  it("names each delete action for its saved query", () => {
    renderBar();

    expect(
      screen
        .getByRole("button", { name: "Delete Current default saved query" })
        .getAttribute("title"),
    ).toBe("Delete Current default saved query");
    expect(
      screen
        .getByRole("button", { name: "Delete Future default saved query" })
        .getAttribute("title"),
    ).toBe("Delete Future default saved query");
  });
});
