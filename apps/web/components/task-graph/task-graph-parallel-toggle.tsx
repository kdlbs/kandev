import { useTranslation } from "react-i18next";
import { IconArrowsSplit2 } from "@tabler/icons-react";
import { Toggle } from "@kandev/ui/toggle";

/** Lays tasks out by step and bands the ones that can run in parallel. */
export function TaskGraphParallelToggle({
  pressed,
  onPressedChange,
}: {
  pressed: boolean;
  onPressedChange: (pressed: boolean) => void;
}) {
  const { t } = useTranslation();
  return (
    <Toggle
      size="sm"
      variant="outline"
      pressed={pressed}
      onPressedChange={onPressedChange}
      data-testid="task-graph-parallel-toggle"
      className="cursor-pointer gap-1.5 px-2.5 [@media(pointer:coarse)]:min-h-11"
    >
      <IconArrowsSplit2 aria-hidden="true" className="size-3.5" />
      {t("task:taskGraphParallelSteps")}
    </Toggle>
  );
}
