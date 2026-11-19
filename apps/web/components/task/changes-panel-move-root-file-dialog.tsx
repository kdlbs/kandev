"use client";

import { useState } from "react";
import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useToast } from "@/components/toast-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { getWebSocketClient } from "@/lib/ws/connection";
import { renameFile } from "@/lib/ws/workspace-files";
import type { WorkspaceRootDestination } from "@/lib/types/workspace-files";
import { useTranslation } from "react-i18next";

function WorkspaceRootDestinationOption({
  destination,
  index,
  path,
  selected,
  touchMode,
}: {
  destination: WorkspaceRootDestination;
  index: number;
  path: string;
  selected: boolean;
  touchMode: boolean;
}) {
  const id = `workspace-root-destination-${index}`;
  return (
    <label
      htmlFor={id}
      className={`flex cursor-pointer items-start gap-3 rounded-md border px-3 py-2 ${touchMode ? "min-h-11" : "min-h-9"} ${selected ? "border-primary bg-accent/40" : "border-border"}`}
    >
      <RadioGroupItem id={id} value={destination.path} className="mt-0.5" />
      <span className="min-w-0 flex-1">
        <span
          className="block truncate text-sm font-medium"
          data-testid="workspace-root-destination-name"
        >
          {destination.repository_name}
        </span>
        <code className="block break-all text-xs text-muted-foreground">
          {destination.path}/{path}
        </code>
      </span>
    </label>
  );
}

function MoveRootFileDialogFooter({
  moveDisabled,
  isMoving,
  onCancel,
  onMove,
}: {
  moveDisabled: boolean;
  isMoving: boolean;
  onCancel: () => void;
  onMove: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DialogFooter>
      <Button variant="outline" onClick={onCancel} disabled={isMoving}>
        {t("common:cancel")}
      </Button>
      <Button onClick={onMove} disabled={moveDisabled}>
        {isMoving && <IconLoader2 className="mr-2 size-4 animate-spin" />}
        {t("task:moveWorkspaceRootFileMove")}
      </Button>
    </DialogFooter>
  );
}

function toastMoveFailure(
  toast: ReturnType<typeof useToast>["toast"],
  title: string,
  description: string,
) {
  toast({ title, description, variant: "error" });
}

export function ChangesPanelMoveRootFileDialog({
  open,
  onOpenChange,
  sessionId,
  path,
  destinations,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  path: string;
  destinations: WorkspaceRootDestination[];
}) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const [selectedPath, setSelectedPath] = useState("");
  const [isMoving, setIsMoving] = useState(false);
  const touchMode = isMobile || !isFinePointer;
  const errorTitle = t("task:moveWorkspaceRootFileError");
  const errorFallback = t("task:moveWorkspaceRootFileErrorFallback");
  const selectedDestination = destinations.find(
    ({ path: destinationPath }) => destinationPath === selectedPath,
  );
  const targetPath = selectedDestination ? `${selectedDestination.path}/${path}` : "";

  const handleOpenChange = (nextOpen: boolean, force = false) => {
    if (!nextOpen && isMoving && !force) return;
    if (!nextOpen) setSelectedPath("");
    onOpenChange(nextOpen);
  };

  const handleMove = async () => {
    if (!selectedDestination || isMoving) return;
    const client = getWebSocketClient();
    if (!client) {
      toastMoveFailure(toast, errorTitle, errorFallback);
      return;
    }

    setIsMoving(true);
    try {
      const response = await renameFile(client, sessionId, path, targetPath);
      if (!response.success) {
        toastMoveFailure(toast, errorTitle, response.error || errorFallback);
        return;
      }
      handleOpenChange(false, true);
    } catch (error) {
      toastMoveFailure(toast, errorTitle, error instanceof Error ? error.message : errorFallback);
    } finally {
      setIsMoving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("task:moveWorkspaceRootFileTitle")}</DialogTitle>
          <DialogDescription>
            {t("task:moveWorkspaceRootFileDescription", { path })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <p className="text-sm font-medium">{t("task:moveWorkspaceRootFileDestination")}</p>
          <RadioGroup value={selectedPath} onValueChange={setSelectedPath} className="space-y-2">
            {destinations.map((destination, index) => {
              return (
                <WorkspaceRootDestinationOption
                  key={destination.path}
                  destination={destination}
                  index={index}
                  path={path}
                  selected={selectedPath === destination.path}
                  touchMode={touchMode}
                />
              );
            })}
          </RadioGroup>
          {selectedDestination && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">
                {t("task:moveWorkspaceRootFileTarget")}
              </p>
              <code className="block break-all rounded-md bg-muted px-2 py-1.5 text-xs">
                {targetPath}
              </code>
            </div>
          )}
        </div>
        <MoveRootFileDialogFooter
          moveDisabled={!selectedDestination || isMoving}
          isMoving={isMoving}
          onCancel={() => handleOpenChange(false)}
          onMove={() => void handleMove()}
        />
      </DialogContent>
    </Dialog>
  );
}
