import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import {
  MobilePRBranchSummary,
  PRSubmitButton,
} from "@/components/task/mobile/session-mobile-top-bar-dialog-parts";
import {
  ChangeRequestPartialStatus,
  PRBranchSummary,
  PRDescriptionField,
  PRTitleField,
} from "./vcs-dialog-fields";

afterEach(cleanup);

describe.each([
  ["gitlab", "Merge Request", "MR"],
  ["github", "Pull Request", "PR"],
])("change request fields for %s", (provider, longName, shortName) => {
  it("uses provider terminology in desktop and mobile controls", () => {
    const terminology = getChangeRequestTerminology(provider);
    render(
      <TooltipProvider>
        <PRTitleField
          prTitle="Title"
          onPrTitleChange={() => {}}
          onGenerateTitle={() => {}}
          isGeneratingTitle={false}
          isUtilityConfigured
          terminology={terminology}
        />
        <PRDescriptionField
          prBody="Body"
          onPrBodyChange={() => {}}
          onGenerateDescription={() => {}}
          isGeneratingDescription={false}
          isUtilityConfigured
          terminology={terminology}
        />
        <PRBranchSummary displayBranch="feature" baseBranch="main" terminology={terminology} />
        <MobilePRBranchSummary
          displayBranch="feature"
          baseBranch="main"
          terminology={terminology}
        />
        <ChangeRequestPartialStatus terminology={terminology} />
        <PRSubmitButton
          prTitle="Title"
          prBody="Body"
          prDraft={false}
          isGitLoading={false}
          onCreatePR={() => {}}
          terminology={terminology}
          branchPushed
        />
      </TooltipProvider>,
    );

    const titleInput = screen.getByLabelText(`${longName} title`) as HTMLInputElement;
    expect(titleInput.placeholder).toBe(`${longName} title...`);
    expect(
      screen.getByRole("button", { name: `Generate ${shortName} title with AI` }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: `Generate ${shortName} description with AI` }),
    ).toBeTruthy();
    expect(screen.getAllByText(new RegExp(`Creating ${shortName} from`))).toHaveLength(2);
    expect(screen.getByRole("status").textContent).toBe(
      `Branch was pushed; retry ${longName.toLowerCase()} creation.`,
    );
    expect(screen.getByRole("button", { name: `Retry ${shortName}` })).toBeTruthy();
  });
});
