"use client";

import { IconAlertTriangle, IconCheck, IconShieldCheck, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import type { GitHubStatus } from "@/lib/types/github";

type PermissionState = {
  name: string;
  available: boolean;
};

function formatCapability(value: string) {
  return value.replaceAll("_", " ").replace(/\b\w/g, (letter) => letter.toUpperCase());
}

function permissionStates(status: GitHubStatus): PermissionState[] {
  const permissions = new Map<string, boolean>(
    Object.entries(status.automation?.capabilities ?? {}),
  );
  for (const permission of [
    ...(status.automation?.missing_capabilities ?? []),
    ...(status.automation?.missing_permissions ?? []),
  ]) {
    permissions.set(permission, false);
  }
  return Array.from(permissions, ([name, available]) => ({ name, available })).sort((a, b) =>
    a.name.localeCompare(b.name),
  );
}

export function GitHubPermissionsDialog({ status }: { status: GitHubStatus }) {
  if (status.automation?.source !== "github_app_installation") return null;
  const permissions = permissionStates(status);
  if (permissions.length === 0) return null;
  const missingCount = permissions.filter((permission) => !permission.available).length;

  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="outline" className="h-11 cursor-pointer">
          {missingCount > 0 ? (
            <IconAlertTriangle className="mr-2 h-4 w-4 text-amber-500" />
          ) : (
            <IconShieldCheck className="mr-2 h-4 w-4" />
          )}
          View permissions
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>GitHub App permissions</DialogTitle>
          <DialogDescription>
            Permissions available to Kandev through this workspace's GitHub App installation.
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[55vh] divide-y overflow-y-auto" data-testid="github-capabilities">
          {permissions.map((permission) => (
            <div key={permission.name} className="flex min-h-11 items-center gap-3 py-2 text-sm">
              {permission.available ? (
                <IconCheck className="h-4 w-4 shrink-0 text-green-500" />
              ) : (
                <IconX className="h-4 w-4 shrink-0 text-destructive" />
              )}
              <span className="min-w-0 flex-1 break-words">
                {formatCapability(permission.name)}
              </span>
              <span
                className={
                  permission.available ? "text-muted-foreground" : "font-medium text-destructive"
                }
              >
                {permission.available ? "Available" : "Missing"}
              </span>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
