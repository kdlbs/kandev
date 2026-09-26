"use client";

import { useState } from "react";
import Link from "@/components/routing/app-link";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { NewTaskDialog } from "@/app/office/components/new-task-dialog";
import { StatusIcon } from "@/app/office/tasks/[id]/status-icon";
import type { Task } from "@/app/office/tasks/[id]/types";
import { useTranslation } from "react-i18next";

type SubIssuesRowProps = {
  task: Task;
};

export function SubIssuesRow({ task }: SubIssuesRowProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <>
      <div className="flex flex-col gap-1 ml-auto items-end">
        {task.children.length === 0 ? (
          <span className="text-muted-foreground text-xs">{t("task:noSubIssues")}</span>
        ) : (
          <ul className="flex flex-col gap-1 w-full" data-testid="sub-issues-list">
            {task.children.map((child) => (
              <li key={child.id}>
                <Link
                  href={`/office/tasks/${child.id}`}
                  className="flex items-center gap-1.5 hover:bg-muted/50 rounded px-1.5 py-0.5 cursor-pointer text-xs"
                >
                  <StatusIcon status={child.status} className="h-3 w-3 shrink-0" />
                  <span className="font-mono text-[10px] text-muted-foreground shrink-0">
                    {child.identifier}
                  </span>
                  <span className="truncate">{child.title}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-xs cursor-pointer"
          onClick={() => setOpen(true)}
          data-testid="sub-issues-add-button"
        >
          <IconPlus className="h-3 w-3 mr-1" />
          {t("task:addSubIssue")}
        </Button>
      </div>
      <NewTaskDialog
        open={open}
        onOpenChange={setOpen}
        parentTaskId={task.id}
        defaultProjectId={task.projectId}
        defaultAssigneeId={task.assigneeAgentProfileId}
      />
    </>
  );
}
