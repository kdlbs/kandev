"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { buildCanvasCreateTaskPrompt } from "./canvas-task-prompt";

export type CanvasTaskCreateLauncherPresentation = "settings" | "sidebar";

export type CanvasTaskCreateLauncherTriggerProps = {
  onOpen: () => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
};

export type CanvasTaskCreateLauncherProps = {
  workspaceId: string | null;
  presentation?: CanvasTaskCreateLauncherPresentation;
  focusReturnRef?: RefObject<HTMLElement | null>;
  children?: (props: CanvasTaskCreateLauncherTriggerProps) => ReactNode;
};

export function CanvasTaskCreateLauncher({
  workspaceId,
  presentation = "settings",
  focusReturnRef,
  children,
}: CanvasTaskCreateLauncherProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const enabled = useFeature("canvases");
  const [open, setOpen] = useState(false);
  const [dialogWorkspaceId, setDialogWorkspaceId] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const focusReturnTargetRef = useMemo<RefObject<HTMLElement | null>>(
    () => ({
      get current() {
        return triggerRef.current ?? focusReturnRef?.current ?? null;
      },
    }),
    [focusReturnRef],
  );

  const handleOpen = useCallback(() => {
    if (!enabled || !workspaceId) return;
    setDialogWorkspaceId(workspaceId);
    setOpen(true);
  }, [enabled, workspaceId]);

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        handleOpen();
        return;
      }
      setOpen(false);
      setDialogWorkspaceId(null);
    },
    [handleOpen],
  );

  useEffect(() => {
    if (!enabled || !workspaceId || (dialogWorkspaceId && dialogWorkspaceId !== workspaceId)) {
      setOpen(false);
      setDialogWorkspaceId(null);
    }
  }, [dialogWorkspaceId, enabled, workspaceId]);

  const handleSuccess = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { willNavigate?: boolean }) => {
      setOpen(false);
      setDialogWorkspaceId(null);
      if (!meta?.willNavigate) router.push(linkToTask(task.id));
    },
    [router],
  );

  if (!enabled || !workspaceId) return null;

  const sidebar = presentation === "sidebar";
  const label = t(sidebar ? "canvases:setUpCanvas" : "canvases:createCanvas");
  const trigger = children ? (
    children({ onOpen: handleOpen, triggerRef })
  ) : (
    <Button
      ref={triggerRef}
      type="button"
      className={
        sidebar
          ? "min-h-8 w-full cursor-pointer justify-start rounded-md px-2.5 py-1.5 text-left text-[13px] font-medium text-muted-foreground hover:bg-muted/60 hover:text-foreground [@media(pointer:coarse)]:min-h-11"
          : controlSizingClassName("standard", "w-full cursor-pointer md:w-auto")
      }
      data-testid={sidebar ? "sidebar-canvases-empty" : "settings-create-canvas"}
      onClick={handleOpen}
    >
      {!sidebar && <IconPlus className="mr-1.5 h-4 w-4" />}
      {label}
    </Button>
  );

  return (
    <>
      {trigger}
      <TaskCreateDialog
        open={open && dialogWorkspaceId === workspaceId}
        onOpenChange={handleOpenChange}
        mode="create"
        workspaceId={workspaceId}
        workflowId={null}
        defaultStepId={null}
        steps={[]}
        initialValues={{
          title: t("canvases:createCanvasTaskTitle"),
          description: buildCanvasCreateTaskPrompt(t("canvases:createCanvasTaskPrompt")),
          noRepository: true,
          preferLocalExecutor: true,
        }}
        focusReturnRef={focusReturnTargetRef}
        onSuccess={handleSuccess}
      />
    </>
  );
}
