"use client";

import { useCallback, useEffect, useState } from "react";
import { IconPlug } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { GitHubStatus } from "@/lib/types/github";
import { GitHubAuthMethodList, type GitHubAutomationMethod } from "./github-auth-method-list";
import { GitHubAppConnectionPanel } from "./github-app-connection-panel";
import { GitHubCLIForm } from "./github-cli-form";
import { GitHubPATForm } from "./github-pat-form";

function methodForStatus(status: GitHubStatus): GitHubAutomationMethod {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
}

const description =
  "This workspace uses one credential for repository sync, watches, background jobs, and managed agent GitHub access. Executor profile credentials can still take precedence.";

function ConnectionBody({
  method,
  workspaceId,
  onMethodChange,
  onSaved,
}: {
  method: GitHubAutomationMethod;
  workspaceId: string;
  onMethodChange: (method: GitHubAutomationMethod) => void;
  onSaved: () => void;
}) {
  return (
    <div className="space-y-5">
      <GitHubAuthMethodList value={method} onChange={onMethodChange} />
      <div className="border-t pt-5">
        {method === "pat" && <GitHubPATForm workspaceId={workspaceId} onSaved={onSaved} />}
        {method === "cli" && <GitHubCLIForm workspaceId={workspaceId} onSaved={onSaved} />}
        {method === "app" && <GitHubAppConnectionPanel workspaceId={workspaceId} />}
      </div>
    </div>
  );
}

export function GitHubConnectionDialog({
  status,
  workspaceId,
  onSaved,
}: {
  status: GitHubStatus;
  workspaceId: string;
  onSaved: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [method, setMethod] = useState<GitHubAutomationMethod>(() => methodForStatus(status));
  const { isMobile } = useResponsiveBreakpoint();
  const connected = Boolean(status.automation);

  useEffect(() => {
    setOpen(false);
    setMethod(methodForStatus(status));
  }, [status, workspaceId]);

  const saved = useCallback(() => {
    onSaved();
    setOpen(false);
  }, [onSaved]);
  const openChange = useCallback(
    (next: boolean) => {
      if (next) setMethod(methodForStatus(status));
      setOpen(next);
    },
    [status],
  );
  const trigger = (
    <Button variant={connected ? "outline" : "default"} className="h-11 cursor-pointer">
      <IconPlug className="mr-2 h-4 w-4" />
      {connected ? "Change connection" : "Connect GitHub"}
    </Button>
  );
  const body = (
    <ConnectionBody
      method={method}
      workspaceId={workspaceId}
      onMethodChange={setMethod}
      onSaved={saved}
    />
  );

  if (isMobile) {
    return (
      <Drawer open={open} onOpenChange={openChange}>
        <DrawerTrigger asChild>{trigger}</DrawerTrigger>
        <DrawerContent
          data-testid="github-connection-mobile"
          className="h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] max-h-[calc(100dvh-16px-env(safe-area-inset-bottom,0px))] overflow-hidden"
        >
          <DrawerHeader className="shrink-0 border-b text-left">
            <DrawerTitle>{connected ? "Change GitHub connection" : "Connect GitHub"}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-4 pb-[calc(1rem+env(safe-area-inset-bottom,0px))] pt-4">
            {body}
          </div>
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={openChange}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent
        data-testid="github-connection-desktop"
        className="flex max-h-[85dvh] flex-col overflow-hidden sm:max-w-2xl"
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>{connected ? "Change GitHub connection" : "Connect GitHub"}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-1">{body}</div>
      </DialogContent>
    </Dialog>
  );
}
