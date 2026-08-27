import { cleanup, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it } from "vitest";
import { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import { VcsChangeRequestDialog } from "./vcs-change-request-dialog";

afterEach(cleanup);

describe.each([
  ["gitlab", "merge request", "MR"],
  ["github", "pull request", "PR"],
])("VcsChangeRequestDialog partial state for %s", (provider, longName, shortName) => {
  it("shows the pushed branch and a provider-aware retry command", () => {
    render(
      <TooltipProvider>
        <VcsChangeRequestDialog
          open
          onOpenChange={() => {}}
          displayBranch="feature"
          baseBranch="main"
          title="Title"
          onTitleChange={() => {}}
          body="Body"
          onBodyChange={() => {}}
          draft={false}
          onDraftChange={() => {}}
          loading={false}
          branchPushed
          onCreate={() => {}}
          onGenerateTitle={() => {}}
          generatingTitle={false}
          onGenerateDescription={() => {}}
          generatingDescription={false}
          utilityConfigured
          terminology={getChangeRequestTerminology(provider)}
        />
      </TooltipProvider>,
    );

    expect(screen.getByRole("status").textContent).toBe(
      `Branch was pushed; retry ${longName} creation.`,
    );
    expect(screen.getByRole("button", { name: `Retry ${shortName}` })).toBeTruthy();
  });
});

describe("VcsChangeRequestDialog provider capabilities", () => {
  it("hides draft creation when the active provider does not support it", () => {
    render(
      <TooltipProvider>
        <VcsChangeRequestDialog
          open
          onOpenChange={() => {}}
          title="Title"
          onTitleChange={() => {}}
          body=""
          onBodyChange={() => {}}
          draft={false}
          onDraftChange={() => {}}
          supportsDraft={false}
          loading={false}
          branchPushed={false}
          onCreate={() => {}}
          onGenerateTitle={() => {}}
          generatingTitle={false}
          onGenerateDescription={() => {}}
          generatingDescription={false}
          utilityConfigured
          terminology={getChangeRequestTerminology("bitbucket")}
        />
      </TooltipProvider>,
    );

    expect(screen.queryByLabelText("Create as draft")).toBeNull();
  });
});
