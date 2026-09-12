import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { RemoteContributionHeaderActions } from "./remote-contribution-header-actions";
import type { RemoteContributionRelation } from "@/hooks/domains/session/remote-contribution-relation";
import type { useRemoteContributionResolution } from "./use-remote-contribution-resolution";

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children, ...props }: React.HTMLAttributes<HTMLDivElement>) => (
    <div {...props}>{children}</div>
  ),
  DropdownMenuItem: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
  DropdownMenuLabel: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSeparator: () => <hr />,
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => (
    <div role="tooltip">{children}</div>
  ),
}));

vi.mock("@/lib/desktop/external-links", () => ({
  openExternalLink: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("./remote-contribution-resolution-dialog", () => ({
  RemoteContributionResolutionDialog: () => null,
}));

afterEach(cleanup);

const relation: RemoteContributionRelation = {
  kind: "diverged",
  presentation: "separate",
  action: "diverged_replace",
  providerHead: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  pushAhead: 0,
  pullBehind: 0,
  canPush: false,
  canPull: false,
  canReplaceRemote: true,
  canUseRemote: true,
};

const HEADER_ACTION_TEST_IDS = {
  compare: "header-compare-versions",
  replace: "header-replace-pr-branch",
  use: "header-use-pr-version",
  view: "header-view-pr-version",
} as const;

function makeResolution() {
  return {
    isLoading: false,
    pending: null,
    errorKey: null,
    lastResult: null,
    requestReplace: vi.fn(),
    requestUse: vi.fn(),
    cancel: vi.fn(),
    confirm: vi.fn(),
  } as unknown as ReturnType<typeof useRemoteContributionResolution>;
}

describe("RemoteContributionHeaderActions", () => {
  it("collapses drift choices behind a warning trigger and explains each action", () => {
    const resolution = makeResolution();
    render(
      <RemoteContributionHeaderActions
        relation={relation}
        resolution={resolution}
        resolutionTarget={{
          repo: "",
          repositoryName: "testorg/testrepo",
          expectedRemoteHead: relation.providerHead!,
        }}
        prUrl="https://github.com/testorg/testrepo/pull/901"
        prNumber={901}
      />,
    );

    const warning = screen.getByTestId("header-remote-contribution-warning");
    expect(warning.getAttribute("aria-label")).toBe("Task and PR histories differ");
    expect(warning.getAttribute("title")).toBe(
      "The task and published PR histories differ. Compare before choosing a version.",
    );
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.replace)).toBeTruthy();
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.use)).toBeTruthy();
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.compare)).toBeTruthy();
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.compare)).toHaveProperty("disabled", true);
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.view).textContent).toContain(
      "Open PR on GitHub",
    );
    expect(
      screen.getByRole("img", { name: /Replace the published PR history/ }).getAttribute("title"),
    ).toBeNull();
    expect(screen.getAllByRole("tooltip").map((tooltip) => tooltip.textContent)).toEqual(
      expect.arrayContaining([
        expect.stringContaining("Replace the published PR history"),
        expect.stringContaining("Replace the task checkout history"),
        expect.stringContaining("Open PR #901 version"),
      ]),
    );

    fireEvent.click(screen.getByTestId("header-replace-pr-branch"));
    fireEvent.click(screen.getByTestId("header-use-pr-version"));
    expect(resolution.requestReplace).toHaveBeenCalledOnce();
    expect(resolution.requestUse).toHaveBeenCalledOnce();
  });

  it("puts comparison before the version changing actions", () => {
    render(
      <RemoteContributionHeaderActions
        relation={relation}
        resolution={makeResolution()}
        resolutionTarget={{
          repo: "",
          repositoryName: "testorg/testrepo",
          expectedRemoteHead: relation.providerHead!,
        }}
        prUrl="https://github.com/testorg/testrepo/pull/901"
        prNumber={901}
      />,
    );
    const menu = screen.getByTestId("header-remote-contribution-menu");
    const itemIds = Array.from(menu.querySelectorAll("[data-testid]")).map((item) =>
      item.getAttribute("data-testid"),
    );
    expect(itemIds.indexOf(HEADER_ACTION_TEST_IDS.compare)).toBeLessThan(
      itemIds.indexOf(HEADER_ACTION_TEST_IDS.replace),
    );
  });

  it("keeps unavailable replacement actions disabled in the history menu", () => {
    render(
      <RemoteContributionHeaderActions
        relation={{ ...relation, canReplaceRemote: false, canUseRemote: false }}
        resolution={makeResolution()}
        resolutionTarget={{
          repo: "",
          repositoryName: "testorg/testrepo",
          expectedRemoteHead: relation.providerHead!,
        }}
        prUrl="https://github.com/testorg/testrepo/pull/901"
        prNumber={901}
      />,
    );

    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.replace)).toHaveProperty("disabled", true);
    expect(screen.getByTestId(HEADER_ACTION_TEST_IDS.use)).toHaveProperty("disabled", true);
  });
});
