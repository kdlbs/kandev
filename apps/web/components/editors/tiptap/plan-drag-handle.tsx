"use client";

import { DragHandle } from "@tiptap/extension-drag-handle-react";
import { IconGripVertical } from "@tabler/icons-react";
import type { Editor } from "@tiptap/core";

type PlanDragHandleProps = {
  editor: Editor;
};

export function PlanDragHandle({ editor }: PlanDragHandleProps) {
  return (
    <DragHandle editor={editor}>
      <div className="plan-drag-handle">
        <IconGripVertical className="h-3.5 w-3.5" />
      </div>
    </DragHandle>
  );
}
