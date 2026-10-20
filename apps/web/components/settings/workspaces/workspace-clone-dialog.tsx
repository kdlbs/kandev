"use client";

import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
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
import {
  settingsActionClassName,
  settingsControlClassName,
} from "@/components/settings/settings-control";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { WorkspaceCloneFlow } from "@/hooks/domains/workspace/use-workspace-clone";

function CloneForm({ flow }: { flow: WorkspaceCloneFlow }) {
  const { t } = useTranslation();
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        void flow.submit();
      }}
      className="flex min-h-0 flex-col"
      aria-busy={flow.pending}
    >
      <div className="min-h-0 space-y-4 overflow-y-auto px-4 py-2 sm:px-0">
        <div className="space-y-2">
          <Label htmlFor="clone-workspace-name">{t("workspaces:workspaceName")}</Label>
          <Input
            id="clone-workspace-name"
            value={flow.name}
            onChange={(event) => flow.setName(event.target.value)}
            required
            autoFocus
            disabled={flow.pending}
            className={settingsControlClassName()}
          />
        </div>
        {flow.error && (
          <p role="alert" className="text-sm text-destructive">
            {flow.error}
          </p>
        )}
      </div>
      <div className="flex shrink-0 flex-col-reverse gap-2 px-4 pt-4 pb-[calc(1rem+env(safe-area-inset-bottom))] sm:flex-row sm:justify-end sm:p-0 sm:pt-4">
        <Button
          type="button"
          variant="outline"
          onClick={flow.close}
          disabled={flow.pending}
          className={settingsActionClassName("max-sm:min-h-12")}
        >
          {t("common:cancel")}
        </Button>
        <Button
          type="submit"
          disabled={flow.pending || !flow.name.trim()}
          className={settingsActionClassName("max-sm:min-h-12")}
        >
          {flow.pending ? t("workspaces:cloningWorkspace") : t("workspaces:cloneWorkspace")}
        </Button>
      </div>
    </form>
  );
}

export function WorkspaceCloneDialog({ flow }: { flow: WorkspaceCloneFlow }) {
  const { t } = useTranslation();
  const { isMobile } = useResponsiveBreakpoint();
  const onOpenChange = (open: boolean) => {
    if (!open) flow.close();
  };
  const title = t("workspaces:cloneWorkspace");
  const description = t("workspaces:cloneDescription", { name: flow.source?.name ?? "" });
  if (isMobile)
    return (
      <Drawer open={Boolean(flow.source)} onOpenChange={onOpenChange} dismissible={!flow.pending}>
        <DrawerContent data-testid="workspace-clone-sheet" onCloseAutoFocus={flow.restoreFocus}>
          <DrawerHeader className="text-left">
            <DrawerTitle>{title}</DrawerTitle>
            <DrawerDescription>{description}</DrawerDescription>
          </DrawerHeader>
          <CloneForm flow={flow} />
        </DrawerContent>
      </Drawer>
    );
  return (
    <Dialog open={Boolean(flow.source)} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-md"
        data-testid="workspace-clone-dialog"
        onCloseAutoFocus={flow.restoreFocus}
        onInteractOutside={(event) => {
          if (flow.pending) event.preventDefault();
        }}
        onEscapeKeyDown={(event) => {
          if (flow.pending) event.preventDefault();
        }}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <CloneForm flow={flow} />
      </DialogContent>
    </Dialog>
  );
}
