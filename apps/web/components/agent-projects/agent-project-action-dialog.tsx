"use client";

import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import {
  projectWorkers,
  useAgentProjectMutations,
} from "@/hooks/domains/agent-projects/use-agent-projects";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { useTranslation } from "react-i18next";

type ProjectAction = "archive" | "delete";

function projectActionCopyKey(action: ProjectAction, workerCount: number): string {
  if (action === "archive") {
    return workerCount === 1
      ? "projects:archiveDescription_one"
      : "projects:archiveDescription_other";
  }
  return workerCount === 1 ? "projects:deleteDescription_one" : "projects:deleteDescription_other";
}

function AgentProjectActionFrame({
  open,
  onOpenChange,
  mobile,
  testId,
  title,
  description,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mobile: boolean;
  testId: string;
  title: string;
  description: string;
  children: ReactNode;
}) {
  if (mobile) {
    return (
      <Drawer direction="bottom" open={open} onOpenChange={onOpenChange}>
        <DrawerContent
          className="flex h-[100dvh] max-h-[100dvh] flex-col rounded-t-none"
          data-testid={testId}
        >
          <DrawerHeader className="shrink-0 text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {children}
        </DrawerContent>
      </Drawer>
    );
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent data-testid={testId}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {children}
      </AlertDialogContent>
    </AlertDialog>
  );
}

function AgentProjectActionBody({
  action,
  deleteContext,
  setDeleteContext,
  discardWorktreeChanges,
  setDiscardWorktreeChanges,
  error,
}: {
  action: ProjectAction;
  deleteContext: boolean;
  setDeleteContext: (value: boolean) => void;
  discardWorktreeChanges: boolean;
  setDiscardWorktreeChanges: (value: boolean) => void;
  error: string | null;
}) {
  const { t } = useTranslation();
  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-3">
      {action === "delete" && (
        <>
          <label className="flex min-h-11 cursor-pointer items-center gap-2 text-sm">
            <Checkbox
              checked={deleteContext}
              onCheckedChange={(checked) => setDeleteContext(checked === true)}
            />
            {t("projects:deleteContext")}
          </label>
          <label className="flex min-h-11 cursor-pointer items-start gap-2 text-sm">
            <Checkbox
              checked={discardWorktreeChanges}
              onCheckedChange={(checked) => setDiscardWorktreeChanges(checked === true)}
            />
            <span>
              <span className="block font-medium">{t("task:discardWorktreeChanges")}</span>
              <span className="block text-xs text-muted-foreground">
                {t("task:discardWorktreeChangesDescription")}
              </span>
            </span>
          </label>
        </>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

function AgentProjectActionFooter({
  action,
  mobile,
  busy,
  onCancel,
  onConfirm,
}: {
  action: ProjectAction;
  mobile: boolean;
  busy: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  const buttonClass = mobile ? "min-h-11 flex-1" : undefined;
  return (
    <>
      <Button
        type="button"
        variant="outline"
        disabled={busy}
        className={buttonClass}
        onClick={onCancel}
      >
        {t("common:cancel")}
      </Button>
      <Button
        type="button"
        variant={action === "delete" ? "destructive" : "default"}
        disabled={busy}
        onClick={onConfirm}
        className={buttonClass}
        data-testid={`agent-project-${action}-confirm`}
      >
        {busy
          ? t("projects:saving")
          : t(action === "archive" ? "projects:archive" : "projects:delete")}
      </Button>
    </>
  );
}

export function AgentProjectActionDialog({
  open,
  onOpenChange,
  workspaceId,
  project,
  action,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  project: AgentProject | null;
  action: ProjectAction;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const mutations = useAgentProjectMutations();
  const [deleteContext, setDeleteContext] = useState(false);
  const [discardWorktreeChanges, setDiscardWorktreeChanges] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (open) {
      setDeleteContext(false);
      setDiscardWorktreeChanges(false);
      setError(null);
    }
  }, [open, project?.id, action]);
  if (!project) return null;

  const run = async () => {
    setBusy(true);
    setError(null);
    try {
      if (action === "archive") await mutations.archive(workspaceId, project.id);
      else await mutations.remove(workspaceId, project.id, deleteContext, discardWorktreeChanges);
      onOpenChange(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally {
      setBusy(false);
    }
  };
  const workerCount = projectWorkers(project).length;

  return (
    <AgentProjectActionFrame
      open={open}
      onOpenChange={onOpenChange}
      mobile={isMobile}
      testId={`agent-project-${action}-confirmation`}
      title={t(action === "archive" ? "projects:archiveTitle" : "projects:deleteTitle", {
        name: project.name,
      })}
      description={t(projectActionCopyKey(action, workerCount), {
        name: project.name,
        count: workerCount,
      })}
    >
      <AgentProjectActionBody
        action={action}
        deleteContext={deleteContext}
        setDeleteContext={setDeleteContext}
        discardWorktreeChanges={discardWorktreeChanges}
        setDiscardWorktreeChanges={setDiscardWorktreeChanges}
        error={error}
      />
      {isMobile ? (
        <DrawerFooter className="shrink-0 flex-row border-t px-4 py-3 pb-[calc(0.75rem+env(safe-area-inset-bottom))]">
          <AgentProjectActionFooter
            action={action}
            mobile={isMobile}
            busy={busy}
            onCancel={() => onOpenChange(false)}
            onConfirm={() => void run()}
          />
        </DrawerFooter>
      ) : (
        <AlertDialogFooter>
          <AgentProjectActionFooter
            action={action}
            mobile={isMobile}
            busy={busy}
            onCancel={() => onOpenChange(false)}
            onConfirm={() => void run()}
          />
        </AlertDialogFooter>
      )}
    </AgentProjectActionFrame>
  );
}
