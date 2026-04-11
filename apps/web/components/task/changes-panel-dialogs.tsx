"use client";

import { useState } from "react";
import { IconGitCommit, IconLoader2, IconCheck } from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from "@kandev/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";

// --- Discard Confirmation Dialog ---

type DiscardDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  fileToDiscard: string | null;
  filesToDiscard: string[] | null;
  onConfirm: () => void;
};

export function DiscardDialog({
  open,
  onOpenChange,
  fileToDiscard,
  filesToDiscard,
  onConfirm,
}: DiscardDialogProps) {
  const isBulk = filesToDiscard && filesToDiscard.length > 1;
  const displayFile = fileToDiscard ?? (filesToDiscard?.length === 1 ? filesToDiscard[0] : null);
  const description = isBulk
    ? `This will permanently discard all changes to ${filesToDiscard.length} files. This action cannot be undone.`
    : null;

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Discard changes?</AlertDialogTitle>
          <AlertDialogDescription>
            {description ?? (
              <>
                This will permanently discard all changes to{" "}
                <span className="font-semibold">{displayFile}</span>. This action cannot be undone.
              </>
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={onConfirm}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Discard
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// --- Amend Commit Message Dialog ---

type AmendDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  amendMessage: string;
  onAmendMessageChange: (value: string) => void;
  onAmend: () => void;
  isLoading: boolean;
};

export function AmendDialog({
  open,
  onOpenChange,
  amendMessage,
  onAmendMessageChange,
  onAmend,
  isLoading,
}: AmendDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <IconGitCommit className="h-5 w-5" />
            Amend Commit Message
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <p className="text-sm text-muted-foreground">
            Edit the message for the most recent commit.
          </p>
          <Textarea
            placeholder="Enter new commit message..."
            value={amendMessage}
            onChange={(e) => onAmendMessageChange(e.target.value)}
            rows={4}
            className="resize-none"
            autoFocus
          />
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" className="cursor-pointer">
              Cancel
            </Button>
          </DialogClose>
          <Button
            onClick={onAmend}
            disabled={!amendMessage.trim() || isLoading}
            className="cursor-pointer"
          >
            {isLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                Amending...
              </>
            ) : (
              <>
                <IconCheck className="h-4 w-4 mr-2" />
                Amend
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// --- Reset to Commit Dialog ---

type ResetDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commitSha: string | null;
  onReset: (mode: "soft" | "hard") => void;
  isLoading: boolean;
};

export function ResetDialog({
  open,
  onOpenChange,
  commitSha,
  onReset,
  isLoading,
}: ResetDialogProps) {
  const shortSha = commitSha?.slice(0, 7) ?? "";
  const [mode, setMode] = useState<"soft" | "hard">("soft");
  const [confirmation, setConfirmation] = useState("");

  const isHardResetConfirmed = mode === "hard" && confirmation === shortSha;
  const canReset = !!commitSha && (mode === "soft" || isHardResetConfirmed);

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset state when closing
      setMode("soft");
      setConfirmation("");
    }
    onOpenChange(newOpen);
  };

  const handleReset = () => {
    if (canReset) {
      onReset(mode);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[450px]">
        <DialogHeader>
          <DialogTitle>Reset to commit {shortSha}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <RadioGroup
            value={mode}
            onValueChange={(v) => setMode(v as "soft" | "hard")}
            className="space-y-3"
          >
            <div className="flex items-start space-x-3">
              <RadioGroupItem value="soft" id="reset-soft" className="mt-1" />
              <div className="flex-1">
                <Label htmlFor="reset-soft" className="font-medium cursor-pointer">
                  Soft Reset
                </Label>
                <p className="text-sm text-muted-foreground">
                  Move HEAD to this commit. Keep all changes staged.
                </p>
              </div>
            </div>
            <div className="flex items-start space-x-3">
              <RadioGroupItem value="hard" id="reset-hard" className="mt-1" />
              <div className="flex-1">
                <Label htmlFor="reset-hard" className="font-medium cursor-pointer text-destructive">
                  Hard Reset
                </Label>
                <p className="text-sm text-muted-foreground">
                  Discard all changes permanently. This cannot be undone.
                </p>
              </div>
            </div>
          </RadioGroup>

          {mode === "hard" && (
            <div className="space-y-2 p-3 border border-destructive/50 rounded-md bg-destructive/5">
              <p className="text-xs text-destructive font-medium">
                Type <code className="bg-muted px-1 rounded">{shortSha}</code> to confirm:
              </p>
              <Input
                value={confirmation}
                onChange={(e) => setConfirmation(e.target.value)}
                placeholder={shortSha}
                className="font-mono h-8 text-sm"
                autoComplete="off"
              />
            </div>
          )}
        </div>
        <DialogFooter>
          <DialogClose asChild>
            <Button type="button" variant="outline" disabled={isLoading} className="cursor-pointer">
              Cancel
            </Button>
          </DialogClose>
          <Button
            onClick={handleReset}
            disabled={!canReset || isLoading}
            variant={mode === "hard" ? "destructive" : "default"}
            className="cursor-pointer"
          >
            {isLoading ? (
              <>
                <IconLoader2 className="h-4 w-4 animate-spin mr-2" />
                Resetting...
              </>
            ) : (
              <>Reset</>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
