"use client";

import { useEffect, useCallback, useRef, useMemo, useState } from "react";
import useEmblaCarousel from "embla-carousel-react";
import { KanbanColumn, WorkflowStep } from "../kanban-column";
import { Task } from "../kanban-card";

type SwipeableColumnsProps = {
  steps: WorkflowStep[];
  tasks: Task[];
  activeIndex: number;
  onIndexChange: (index: number) => void;
  onPreviewTask: (task: Task) => void;
  onOpenTask: (task: Task) => void;
  onEditTask: (task: Task) => void;
  onDeleteTask: (task: Task) => void;
  onArchiveTask?: (task: Task) => void;
  onMoveTask?: (task: Task, targetStepId: string) => void;
  showMaximizeButton?: boolean;
  deletingTaskId?: string | null;
  archivingTaskId?: string | null;
};

export function SwipeableColumns({
  steps,
  tasks,
  activeIndex,
  onIndexChange,
  onPreviewTask,
  onOpenTask,
  onEditTask,
  onDeleteTask,
  onArchiveTask,
  onMoveTask,
  showMaximizeButton,
  deletingTaskId,
  archivingTaskId,
}: SwipeableColumnsProps) {
  // Stable options to avoid Embla reinitializing on every activeIndex change
  const [initialIndex] = useState(activeIndex);
  const options = useMemo(
    () => ({
      align: "start" as const,
      containScroll: "trimSnaps" as const,
      watchDrag: true,
      startIndex: initialIndex,
    }),
    [initialIndex],
  );
  const [emblaRef, emblaApi] = useEmblaCarousel(options);
  const userInteracting = useRef(false);

  const getTasksForStep = useCallback(
    (stepId: string) => {
      return tasks
        .filter((task) => task.workflowStepId === stepId)
        .map((task) => ({ ...task, position: task.position ?? 0 }))
        .sort((a, b) => (a.createdAt ?? "").localeCompare(b.createdAt ?? ""));
    },
    [tasks],
  );

  // Sync carousel position with external activeIndex (tab clicks)
  useEffect(() => {
    if (emblaApi && emblaApi.selectedScrollSnap() !== activeIndex) {
      emblaApi.scrollTo(activeIndex, true);
    }
  }, [emblaApi, activeIndex]);

  // Sync tab state when user swipes (only on user-initiated interactions)
  useEffect(() => {
    if (!emblaApi) return;

    const onPointerDown = () => {
      userInteracting.current = true;
    };
    const onSelect = () => {
      if (!userInteracting.current) return;
      userInteracting.current = false;
      const selectedIndex = emblaApi.selectedScrollSnap();
      if (selectedIndex !== activeIndex) {
        onIndexChange(selectedIndex);
      }
    };

    emblaApi.on("pointerDown", onPointerDown);
    emblaApi.on("select", onSelect);
    return () => {
      emblaApi.off("pointerDown", onPointerDown);
      emblaApi.off("select", onSelect);
    };
  }, [emblaApi, activeIndex, onIndexChange]);

  // Use explicit height calculation for mobile since flex height inheritance doesn't work reliably
  // 100dvh - header (~56px) - tabs (~44px) - safe area
  return (
    <div
      className="flex-1 min-h-0 overflow-hidden"
      ref={emblaRef}
      style={{ height: "calc(100dvh - 100px - env(safe-area-inset-bottom, 0px))" }}
    >
      <div className="flex h-full touch-pan-y">
        {steps.map((step) => (
          <div
            key={step.id}
            className="flex-shrink-0 w-full h-full min-w-0 px-4 py-2 flex flex-col"
          >
            <KanbanColumn
              step={step}
              tasks={getTasksForStep(step.id)}
              onPreviewTask={onPreviewTask}
              onOpenTask={onOpenTask}
              onEditTask={onEditTask}
              onDeleteTask={onDeleteTask}
              onArchiveTask={onArchiveTask}
              onMoveTask={onMoveTask}
              steps={steps}
              showMaximizeButton={showMaximizeButton}
              deletingTaskId={deletingTaskId}
              archivingTaskId={archivingTaskId}
              hideHeader
            />
          </div>
        ))}
      </div>
    </div>
  );
}
