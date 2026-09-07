"use client";

import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import type { Task } from "@/lib/types/http";

export type CanvasTaskCreateLauncherProps = {
  workspaceId: string | null;
};

export function CanvasTaskCreateLauncher({ workspaceId }: CanvasTaskCreateLauncherProps) {
  const { t } = useTranslation();
  const router = useRouter();
  const enabled = useFeature("canvases");
  const [open, setOpen] = useState(false);

  const handleSuccess = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { willNavigate?: boolean }) => {
      setOpen(false);
      if (!meta?.willNavigate) router.push(linkToTask(task.id));
    },
    [router],
  );

  if (!enabled || !workspaceId) return null;

  const label = t("canvases:createCanvas");

  return (
    <>
      <Button
        type="button"
        className="min-h-11 w-full cursor-pointer md:min-h-7 md:w-auto"
        data-testid="settings-create-canvas"
        onClick={() => setOpen(true)}
      >
        <IconPlus className="mr-1.5 h-4 w-4" />
        {label}
      </Button>
      <TaskCreateDialog
        open={open}
        onOpenChange={setOpen}
        mode="create"
        workspaceId={workspaceId}
        workflowId={null}
        defaultStepId={null}
        steps={[]}
        initialValues={{
          title: t("canvases:createCanvasTaskTitle"),
          description: t("canvases:createCanvasTaskPrompt"),
          noRepository: true,
          preferLocalExecutor: true,
        }}
        onSuccess={handleSuccess}
      />
    </>
  );
}
