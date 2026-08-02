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
import type { TaskGitCredentialsState } from "@/hooks/domains/github/use-task-git-credentials";
import type { GitHubStatus } from "@/lib/types/github";
import type { GitHubAutomationMethod } from "./github-auth-method-list";
import { GitHubConnectionSettingsForm } from "./github-connection-settings-form";
import { useTranslation } from "react-i18next";

function methodForStatus(status: GitHubStatus): GitHubAutomationMethod {
  if (status.automation?.source === "github_app_installation") return "app";
  if (status.automation?.source === "gh_cli") return "cli";
  return "pat";
}

export function GitHubConnectionDialog({
  status,
  workspaceId,
  onSaved,
  taskAccess,
}: {
  status: GitHubStatus;
  workspaceId: string;
  onSaved: () => void;
  taskAccess: TaskGitCredentialsState;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [method, setMethod] = useState<GitHubAutomationMethod>(() => methodForStatus(status));
  const { isMobile } = useResponsiveBreakpoint();
  const connected = Boolean(status.automation);

  useEffect(() => {
    setMethod(methodForStatus(status));
  }, [status]);

  useEffect(() => {
    setOpen(false);
  }, [workspaceId]);

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
      {connected ? t("github:changeConnection") : t("github:connectGithub")}
    </Button>
  );
  const body = (
    <GitHubConnectionSettingsForm
      status={status}
      method={method}
      workspaceId={workspaceId}
      open={open}
      onMethodChange={setMethod}
      onSaved={onSaved}
      onComplete={() => setOpen(false)}
      taskAccess={taskAccess}
      isMobile={isMobile}
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
            <DrawerTitle>
              {connected ? t("github:changeGithubConnection") : t("github:connectGithub")}
            </DrawerTitle>
            <DrawerDescription>{t("github:thisWorkspaceUsesOneCredentialFor")}</DrawerDescription>
          </DrawerHeader>
          {body}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <Dialog open={open} onOpenChange={openChange}>
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent
        data-testid="github-connection-desktop"
        className="flex max-h-[85dvh] flex-col overflow-hidden sm:max-w-4xl"
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>
            {connected ? t("github:changeGithubConnection") : t("github:connectGithub")}
          </DialogTitle>
          <DialogDescription>{t("github:thisWorkspaceUsesOneCredentialFor")}</DialogDescription>
        </DialogHeader>
        {body}
      </DialogContent>
    </Dialog>
  );
}
