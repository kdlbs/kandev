"use client";

import type { ReactNode } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@kandev/ui/drawer";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { useTranslation } from "react-i18next";
import { useAgentProjectFormController } from "./agent-project-form-controller";
import { ProjectFormBody, useProjectFormData } from "./agent-project-form-fields";

function AgentProjectFormFrame({
  open,
  onOpenChange,
  mobile,
  title,
  description,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mobile: boolean;
  title: string;
  description: string;
  children: ReactNode;
}) {
  if (mobile) {
    return (
      <Drawer
        open={open}
        onOpenChange={onOpenChange}
        direction="bottom"
        shouldScaleBackground={false}
      >
        <DrawerContent
          className="!inset-x-0 !bottom-0 !h-[100dvh] !max-h-[100dvh] !rounded-none !p-0"
          data-testid="agent-project-form-mobile"
        >
          <DrawerHeader className="shrink-0 border-b text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          {children}
        </DrawerContent>
      </Drawer>
    );
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex max-h-[min(85dvh,760px)] flex-col overflow-hidden sm:max-w-xl"
        data-testid="agent-project-form-desktop"
      >
        <DialogHeader className="shrink-0">
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}

export function AgentProjectFormDialog({
  open,
  onOpenChange,
  workspaceId,
  project,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  project?: AgentProject;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const formData = useProjectFormData(open, workspaceId, project);
  const form = useAgentProjectFormController(open, workspaceId, project, formData, onOpenChange);
  const title = t(project ? "projects:editTitle" : "projects:createTitle");

  return (
    <AgentProjectFormFrame
      open={open}
      onOpenChange={onOpenChange}
      mobile={isMobile}
      title={title}
      description={t("projects:formDescription")}
    >
      <ProjectFormBody
        onSubmit={(event) => void form.submit(event)}
        formData={formData}
        draft={form.draft}
        mobile={isMobile}
        ready={form.ready}
        error={form.error}
        saving={form.saving}
        project={project}
        applyDefaultExecutor={form.applyDefaultExecutor}
        onNameChange={(name) => form.updateDraft({ name })}
        onToggleRepository={form.toggleRepository}
        onPrimaryRepositoryChange={(primaryRepositoryId) =>
          form.updateDraft({ primaryRepositoryId })
        }
        onDraftChange={form.updateDraft}
        onApplyDefaultExecutorChange={form.setApplyDefaultExecutor}
        onClose={() => onOpenChange(false)}
      />
    </AgentProjectFormFrame>
  );
}
