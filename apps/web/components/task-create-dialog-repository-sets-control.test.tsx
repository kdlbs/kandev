import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";

import type { Repository, RepositorySet } from "@/lib/types/http";
import { repositoryId, workspaceId } from "@/lib/types/ids";
import {
  RepositorySetMenuItems,
  RepositorySetsControl,
} from "@/components/task-create-dialog-repository-sets-control";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";

function repository(id: string): Repository {
  return { id: repositoryId(id), name: id } as unknown as Repository;
}

const REPO_WEB = "repo-web";
const REPO_GATEWAY = "repo-gateway";
const OPTION_TESTID = "repository-set-option";
const TRIGGER_TESTID = "repository-sets-trigger";
const FULL_STACK = "Full-stack";
const BACKEND = "Backend";
const AVAILABLE = [repository(REPO_WEB), repository(REPO_GATEWAY)];

function repositorySet(id: string, name: string, ids: string[]): RepositorySet {
  return {
    id,
    workspace_id: workspaceId("ws-1"),
    name,
    description: "",
    repositories: ids.map((member, position) => ({
      repository_id: repositoryId(member),
      position,
    })),
    created_at: "2026-08-17T09:00:00Z",
    updated_at: "2026-08-17T09:00:00Z",
  };
}

// Listed in the order the store keeps them: by name, case-insensitively.
const SETS = [
  repositorySet("set-2", BACKEND, [REPO_GATEWAY]),
  repositorySet("set-1", FULL_STACK, [REPO_WEB, REPO_GATEWAY]),
];

const PLACEHOLDER_ROW: TaskRepoRow = { key: "row-0", branch: "" };

/**
 * Renders the menu body inside an open menu. Radix opens on pointer events that
 * `fireEvent.click` does not produce, so every menu-content test in this repo
 * mounts the content already open instead.
 */
function renderMenu(
  overrides: {
    sets?: RepositorySet[];
    repositories?: Repository[];
    rows?: TaskRepoRow[];
  } = {},
) {
  const onApply = vi.fn();
  render(
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger>open</DropdownMenuTrigger>
      <DropdownMenuContent>
        <RepositorySetMenuItems
          sets={overrides.sets ?? SETS}
          repositories={overrides.repositories ?? AVAILABLE}
          rows={overrides.rows ?? [PLACEHOLDER_ROW]}
          onApply={onApply}
        />
      </DropdownMenuContent>
    </DropdownMenu>,
  );
  return { onApply };
}

function optionNamed(name: string): HTMLElement {
  const match = screen
    .getAllByTestId(OPTION_TESTID)
    .find((item) => item.textContent?.includes(name));
  if (!match) throw new Error(`no option named ${name}`);
  return match;
}

afterEach(() => cleanup());

describe("RepositorySetsControl trigger", () => {
  it("renders nothing when the workspace has no sets and there is no other action", () => {
    render(
      <RepositorySetsControl
        sets={[]}
        repositories={AVAILABLE}
        rows={[PLACEHOLDER_ROW]}
        onApply={vi.fn()}
      />,
    );
    expect(screen.queryByTestId(TRIGGER_TESTID)).toBeNull();
  });

  it("still renders when there are no sets but a footer action is offered", () => {
    render(
      <RepositorySetsControl
        sets={[]}
        repositories={AVAILABLE}
        rows={[PLACEHOLDER_ROW]}
        onApply={vi.fn()}
        footerActions={<span>save as set</span>}
      />,
    );
    expect(screen.queryByTestId(TRIGGER_TESTID)).not.toBeNull();
  });

  it("never disables itself on executor capability", () => {
    // "Add repository" is not executor-gated either: the repository selection
    // constrains which executor profiles are offered, not the reverse. Disabling
    // this control instead wedged a long sentence into the chip row, and Radix
    // kept the menu openable anyway because DropdownMenuTrigger owns its own
    // pointer handlers.
    render(
      <RepositorySetsControl
        sets={SETS}
        repositories={AVAILABLE}
        rows={[PLACEHOLDER_ROW]}
        onApply={vi.fn()}
      />,
    );

    const trigger = screen.getByTestId(TRIGGER_TESTID) as HTMLButtonElement;
    expect(trigger.disabled).toBe(false);
  });

  it("renders only the trigger, with no explanatory sentence beside it", () => {
    const { container } = render(
      <RepositorySetsControl
        sets={SETS}
        repositories={AVAILABLE}
        rows={[PLACEHOLDER_ROW]}
        onApply={vi.fn()}
      />,
    );

    // The chip row is crowded; the control contributes its own label and nothing
    // else.
    expect(container.textContent?.trim()).toBe("Sets");
  });
});

describe("RepositorySetMenuItems", () => {
  it("lists each set with its repository count", () => {
    renderMenu();

    const items = screen.getAllByTestId(OPTION_TESTID);
    expect(items).toHaveLength(2);
    expect(items[0].textContent).toContain(BACKEND);
    expect(items[0].textContent).toContain("1 repository");
    expect(items[1].textContent).toContain(FULL_STACK);
    expect(items[1].textContent).toContain("2 repositories");
  });

  it("applies the chosen set", () => {
    const { onApply } = renderMenu();

    fireEvent.click(optionNamed(FULL_STACK));

    expect(onApply).toHaveBeenCalledTimes(1);
    expect(onApply.mock.calls[0][0].name).toBe(FULL_STACK);
  });

  it("reports how many members a set would skip as no longer available", () => {
    renderMenu({ repositories: [repository(REPO_WEB)] });

    expect(optionNamed(FULL_STACK).textContent).toContain("1 no longer available");
    // A set whose only member is gone says so too.
    expect(optionNamed(BACKEND).textContent).toContain("1 no longer available");
  });

  it("marks a set whose members are all already in the form", () => {
    renderMenu({ rows: [{ key: "row-0", repositoryId: REPO_GATEWAY, branch: "main" }] });

    expect(optionNamed(BACKEND).getAttribute("data-fully-applied")).toBe("true");
    expect(optionNamed(BACKEND).textContent).toContain("already added");
  });

  it("does not mark a set that would still add a row", () => {
    renderMenu({ rows: [{ key: "row-0", repositoryId: REPO_GATEWAY, branch: "main" }] });

    expect(optionNamed(FULL_STACK).getAttribute("data-fully-applied")).toBe("false");
  });

  it("renders footer actions under a separator when sets exist", () => {
    render(
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger>open</DropdownMenuTrigger>
        <DropdownMenuContent>
          <RepositorySetMenuItems
            sets={SETS}
            repositories={AVAILABLE}
            rows={[PLACEHOLDER_ROW]}
            onApply={vi.fn()}
            footerActions={<span data-testid="footer-action">save as set</span>}
          />
        </DropdownMenuContent>
      </DropdownMenu>,
    );

    expect(screen.queryByTestId("footer-action")).not.toBeNull();
  });
});
