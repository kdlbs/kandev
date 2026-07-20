"use client";

import { IconGitPullRequest, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Label } from "@kandev/ui/label";
import type { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import {
  ChangeRequestPartialStatus,
  PRBranchSummary,
  PRDescriptionField,
  PRTitleField,
} from "./vcs-dialog-fields";

type VcsChangeRequestDialogProps = {
  open: boolean;
  onOpenChange: (value: boolean) => void;
  scopedRepo?: string;
  displayBranch?: string | null;
  baseBranch?: string;
  title: string;
  onTitleChange: (value: string) => void;
  body: string;
  onBodyChange: (value: string) => void;
  draft: boolean;
  onDraftChange: (value: boolean) => void;
  loading: boolean;
  branchPushed: boolean;
  onCreate: () => void;
  onGenerateTitle: () => void;
  generatingTitle: boolean;
  onGenerateDescription: () => void;
  generatingDescription: boolean;
  utilityConfigured: boolean;
  terminology: ReturnType<typeof getChangeRequestTerminology>;
};

export function VcsChangeRequestDialog(props: VcsChangeRequestDialogProps) {
  const terms = props.terminology;
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitPullRequest className="h-5 w-5" />
            {props.scopedRepo
              ? `Create ${terms.longName} — ${props.scopedRepo}`
              : `Create ${terms.longName}`}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {props.branchPushed && <ChangeRequestPartialStatus terminology={terms} />}
          <PRBranchSummary
            displayBranch={props.displayBranch}
            baseBranch={props.baseBranch}
            terminology={terms}
          />
          <PRTitleField
            prTitle={props.title}
            onPrTitleChange={props.onTitleChange}
            onGenerateTitle={props.onGenerateTitle}
            isGeneratingTitle={props.generatingTitle}
            isUtilityConfigured={props.utilityConfigured}
            terminology={terms}
          />
          <PRDescriptionField
            prBody={props.body}
            onPrBodyChange={props.onBodyChange}
            onGenerateDescription={props.onGenerateDescription}
            isGeneratingDescription={props.generatingDescription}
            isUtilityConfigured={props.utilityConfigured}
            terminology={terms}
          />
          <div className="flex items-center space-x-2">
            <Checkbox
              id="vcs-pr-draft"
              checked={props.draft}
              onCheckedChange={(checked) => props.onDraftChange(checked === true)}
            />
            <Label htmlFor="vcs-pr-draft" className="text-sm cursor-pointer">
              Create as draft
            </Label>
          </div>
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              Cancel
            </Button>
          </DialogClose>
          <Button onClick={props.onCreate} disabled={!props.title.trim() || props.loading}>
            {props.loading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                Creating...
              </>
            ) : (
              <>
                <IconGitPullRequest className="h-4 w-4 mr-2" />
                {props.branchPushed ? "Retry" : "Create"} {terms.shortName}
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
