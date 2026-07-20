"use client";

import { IconCheck, IconShieldCheck } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";

const permissions = [
  ["Actions", "Read workflow runs and artifacts"],
  ["Administration", "Read branch protection"],
  ["Checks", "Read check runs"],
  ["Contents", "Read and write repository content"],
  ["Issues", "Read and write issues"],
  ["Members", "Read organization membership"],
  ["Metadata", "Read repository metadata"],
  ["Pull requests", "Read and write pull requests"],
  ["Commit statuses", "Read commit statuses"],
  ["Workflows", "Write workflow files"],
] as const;

export function GitHubAppPermissionsDialog() {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 w-full cursor-pointer sm:w-auto"
          data-testid="github-app-permissions-button"
        >
          <IconShieldCheck className="mr-2 h-4 w-4" />
          Review permissions
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-hidden sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>GitHub App permissions</DialogTitle>
          <DialogDescription>
            Kandev generates this policy. Future permission increases require installation owners to
            approve the update on GitHub.
          </DialogDescription>
        </DialogHeader>
        <div
          className="min-h-0 max-h-[60dvh] divide-y overflow-y-auto overscroll-contain"
          data-testid="github-app-permissions-list"
        >
          {permissions.map(([name, detail]) => (
            <div key={name} className="flex min-h-11 items-start gap-3 py-2.5 text-sm">
              <IconCheck className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
              <div className="min-w-0">
                <p className="font-medium">{name}</p>
                <p className="text-xs text-muted-foreground">{detail}</p>
              </div>
            </div>
          ))}
        </div>
        <p className="text-xs text-muted-foreground">
          Webhooks: installation, installation repositories, and GitHub App authorization.
        </p>
      </DialogContent>
    </Dialog>
  );
}
