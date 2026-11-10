"use client";

import { memo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { TaskFormInputs } from "@/components/task-create-dialog-selectors";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { TaskCreateAttachmentController } from "@/hooks/domains/task/use-task-create-attachments";

type Props = {
  workspaceId: string;
  description: string;
  controller: TaskCreateAttachmentController;
  disabled?: boolean;
  onDescriptionChange: (value: string) => void;
};

export const NewTaskAttachmentComposer = memo(function NewTaskAttachmentComposer({
  workspaceId,
  description,
  controller,
  disabled,
  onDescriptionChange,
}: Props) {
  const { t } = useTranslation();
  const descriptionRef = useRef<TaskFormInputsHandle | null>(null);
  return (
    <TaskFormInputs
      isSessionMode={false}
      workspaceId={workspaceId}
      initialDescription={description}
      onDescriptionChange={() => undefined}
      onDescriptionValueChange={onDescriptionChange}
      onKeyDown={() => undefined}
      descriptionValueRef={descriptionRef}
      attachmentController={controller}
      placeholder={t("office:addDescription")}
      disabled={disabled}
    />
  );
});
