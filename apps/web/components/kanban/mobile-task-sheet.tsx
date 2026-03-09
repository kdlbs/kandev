"use client";

import { Button } from "@kandev/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
  DrawerFooter,
} from "@kandev/ui/drawer";
import { Badge } from "@kandev/ui/badge";
import { IconArrowRight, IconEdit, IconTrash } from "@tabler/icons-react";
import type { Task } from "@/components/kanban-card";

type MobileTaskSheetProps = {
  task: Task | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onGoToSession: (task: Task) => void;
  onEdit: (task: Task) => void;
  onDelete: (task: Task) => void;
};

export function MobileTaskSheet({
  task,
  open,
  onOpenChange,
  onGoToSession,
  onEdit,
  onDelete,
}: MobileTaskSheetProps) {
  if (!task) return null;

  return (
    <Drawer open={open} onOpenChange={onOpenChange}>
      <DrawerContent data-testid="mobile-task-sheet">
        <DrawerHeader className="text-left">
          <DrawerTitle className="text-base">{task.title}</DrawerTitle>
          {task.description && (
            <DrawerDescription className="line-clamp-3">{task.description}</DrawerDescription>
          )}
          <div className="flex items-center gap-2 mt-1">
            <Badge variant="secondary" className="text-xs">
              {task.state ?? "not_started"}
            </Badge>
          </div>
        </DrawerHeader>
        <DrawerFooter className="flex-row gap-2">
          <Button
            variant="outline"
            size="sm"
            className="flex-1 cursor-pointer"
            onClick={() => {
              onEdit(task);
              onOpenChange(false);
            }}
          >
            <IconEdit className="h-4 w-4 mr-1.5" />
            Edit
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="flex-1 cursor-pointer text-destructive hover:text-destructive"
            onClick={() => {
              onDelete(task);
              onOpenChange(false);
            }}
          >
            <IconTrash className="h-4 w-4 mr-1.5" />
            Delete
          </Button>
          <Button
            size="sm"
            className="flex-[2] cursor-pointer"
            onClick={() => {
              onGoToSession(task);
              onOpenChange(false);
            }}
          >
            <IconArrowRight className="h-4 w-4 mr-1.5" />
            Open Session
          </Button>
        </DrawerFooter>
      </DrawerContent>
    </Drawer>
  );
}
