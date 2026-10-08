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
import { ProjectHelpButton } from "./agent-project-form-field-controls";

function AgentProjectFormFrame({
  open,
  onOpenChange,
  mobile,
  title,
  projectHelpLabel,
  projectHelpDescription,
  description,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mobile: boolean;
  title: string;
  projectHelpLabel: string;
  projectHelpDescription: string;
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
          className="!inset-x-0 !bottom-0 !h-[100dvh] !max-h-[100dvh] !rounded-none !p-0 before:inset-0 before:rounded-none [&>div:first-child]:hidden"
          data-testid="agent-project-form-mobile"
        >
          <DrawerHeader className="shrink-0 border-b group-data-[vaul-drawer-direction=bottom]/drawer-content:text-left">
            <div className="flex items-center gap-1">
              <DrawerTitle>{title}</DrawerTitle>
              <ProjectHelpButton
                label={projectHelpLabel}
                description={projectHelpDescription}
                testId="agent-project-project-help"
              />
            </div>
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
        className="flex max-h-[min(85dvh,760px)] flex-col overflow-hidden sm:max-w-[900px] [&>[data-slot=dialog-close]]:[@media(pointer:coarse)]:size-11"
        data-testid="agent-project-form-desktop"
      >
        <DialogHeader className="shrink-0 [@media(pointer:coarse)]:pr-9">
          <div className="flex items-center gap-1">
            <DialogTitle>{title}</DialogTitle>
            <ProjectHelpButton
              label={projectHelpLabel}
              description={projectHelpDescription}
              testId="agent-project-project-help"
            />
          </div>
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
  onOpenCoordinator,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
  project?: AgentProject;
  onOpenCoordinator?: (taskId: string) => void;
}) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const formData = useProjectFormData(open, workspaceId, project);
  const form = useAgentProjectFormController({
    open,
    workspaceId,
    project,
    formData,
    onOpenChange,
    onOpenCoordinator,
  });
  const title = t(project ? "projects:editTitle" : "projects:createTitle");

  return (
    <AgentProjectFormFrame
      open={open}
      onOpenChange={form.handleOpenChange}
      mobile={isMobile}
      title={title}
      projectHelpLabel={t("projects:projectHelpLabel")}
      projectHelpDescription={t("projects:projectHelp")}
      description={t("projects:formDescription")}
    >
      <ProjectFormBody
        workspaceId={workspaceId}
        onSubmit={(event) => void form.submit(event)}
        formData={formData}
        draft={form.draft}
        mobile={isMobile}
        ready={form.ready}
        error={form.error}
        saving={form.saving}
        project={project}
        createdProject={form.createdProject}
        createRetryLocked={form.createRetryLocked}
        promptLocked={form.promptLocked}
        hasPrompt={form.draft.initialPrompt.trim().length > 0}
        applyDefaultExecutor={form.applyDefaultExecutor}
        onNameChange={(name) => form.updateDraft({ name })}
        onPromptChange={(initialPrompt) => form.updateDraft({ initialPrompt })}
        onToggleRepository={form.toggleRepository}
        onSelectRemoteRepository={form.selectRemoteRepository}
        onPrimaryRepositoryChange={(primaryRepositoryId) =>
          form.updateDraft({ primaryRepositoryId })
        }
        onDraftChange={form.updateDraft}
        onApplyDefaultExecutorChange={form.setApplyDefaultExecutor}
        onClose={form.closeDialog}
        onRetryStart={() => void form.retryStart()}
        onOpenCoordinator={form.openCoordinator}
      />
    </AgentProjectFormFrame>
  );
}
