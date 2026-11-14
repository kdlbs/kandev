import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { stepFrames, type TaskGraphLayout } from "@/lib/task-graph/layout";
import type { TaskGraphModel } from "@/lib/task-graph/model";
import type { TaskGraphFrame } from "./task-graph-renderer";

/** Labelled step bands, or none when the toggle is off. */
export function useStepFrames(
  model: TaskGraphModel,
  layout: TaskGraphLayout,
  enabled: boolean,
): TaskGraphFrame[] {
  const { t } = useTranslation();
  return useMemo(() => {
    if (!enabled) return [];
    return stepFrames(model, layout).map((frame) => ({
      id: `step:${frame.step}`,
      title:
        frame.count > 1
          ? t("task:taskGraphStepParallel", { step: frame.step, count: frame.count })
          : t("task:taskGraphStepLabel", { step: frame.step }),
      box: frame.box,
    }));
  }, [model, layout, enabled, t]);
}
