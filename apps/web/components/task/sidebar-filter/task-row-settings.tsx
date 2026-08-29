"use client";

import { useMemo, useState } from "react";
import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  TouchSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { IconChevronDown, IconGripVertical } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import type { SortSpec } from "@/lib/state/slices/ui/sidebar-view-types";
import {
  SIDEBAR_TASK_ROW_TRAILING_KEYS,
  type SidebarTaskRowDetail,
  type SidebarTaskRowPresentation,
  type SidebarTaskRowTrailing,
} from "@/lib/state/slices/ui/sidebar-task-row-presentation";

const DETAIL_LABEL_KEYS: Record<SidebarTaskRowDetail, string> = {
  relative_time: "task:taskRowRelativeTime",
  repository: "task:taskRowRepository",
  pull_request_number: "task:taskRowPullRequestNumber",
};

const TRAILING_LABEL_KEYS: Record<SidebarTaskRowTrailing, string> = {
  git_changes: "task:taskRowGitChanges",
  relative_time: "task:taskRowRelativeTime",
  change_request_status: "task:taskRowChangeRequestStatus",
  none: "task:taskRowNothing",
};

const TRAILING_DESCRIPTION_KEYS: Record<SidebarTaskRowTrailing, string> = {
  git_changes: "task:taskRowGitChangesDescription",
  relative_time: "task:taskRowRelativeTimeDescription",
  change_request_status: "task:taskRowChangeRequestStatusDescription",
  none: "task:taskRowNothingDescription",
};

function TaskRowSwitch({
  id,
  checked,
  onCheckedChange,
  ariaLabel,
  testId,
}: {
  id: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  ariaLabel: string;
  testId: string;
}) {
  return (
    <label
      htmlFor={id}
      className="flex size-11 shrink-0 cursor-pointer items-center justify-center"
      data-testid={testId}
    >
      <Switch
        id={id}
        size="sm"
        checked={checked}
        onCheckedChange={onCheckedChange}
        aria-label={ariaLabel}
        className="after:-inset-y-4"
      />
    </label>
  );
}

export function reorderSidebarTaskRowDetails(
  order: SidebarTaskRowDetail[],
  activeId: SidebarTaskRowDetail,
  overId: SidebarTaskRowDetail,
): SidebarTaskRowDetail[] {
  const oldIndex = order.indexOf(activeId);
  const newIndex = order.indexOf(overId);
  if (oldIndex < 0 || newIndex < 0 || oldIndex === newIndex) return order;
  return arrayMove(order, oldIndex, newIndex);
}

function SortableDetailRow({
  detail,
  value,
  onToggle,
}: {
  detail: SidebarTaskRowDetail;
  value: SidebarTaskRowPresentation;
  onToggle: (detail: SidebarTaskRowDetail, checked: boolean) => void;
}) {
  const { t } = useTranslation();
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: detail,
  });
  const label = t(DETAIL_LABEL_KEYS[detail]);

  return (
    <div
      ref={setNodeRef}
      data-testid={`task-row-detail-${detail}`}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className="flex min-h-11 items-center gap-1 rounded-md border-b border-border/40 last:border-b-0"
      data-dragging={isDragging ? "true" : undefined}
    >
      <button
        type="button"
        aria-label={t("task:taskRowReorderHandle", { label })}
        className="flex min-h-11 min-w-11 shrink-0 touch-none items-center justify-center text-muted-foreground/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        data-testid={`task-row-detail-handle-${detail}`}
        {...attributes}
        {...listeners}
        aria-roledescription={t("task:taskRowReorderable")}
      >
        <IconGripVertical className="size-4" aria-hidden="true" />
      </button>
      <label htmlFor={`task-row-detail-${detail}-toggle`} className="min-w-0 flex-1 py-2">
        <span className="flex flex-wrap items-baseline gap-x-2 text-xs font-medium">
          <span>{label}</span>
          {detail === "relative_time" && value.trailing === "relative_time" && (
            <span
              className="text-[11px] font-normal text-muted-foreground"
              data-testid="task-row-relative-time-shown-on-right"
            >
              {t("task:taskRowShownOnRight")}
            </span>
          )}
        </span>
      </label>
      <TaskRowSwitch
        id={`task-row-detail-${detail}-toggle`}
        checked={value.visibleDetails.includes(detail)}
        onCheckedChange={(checked) => onToggle(detail, checked)}
        ariaLabel={t("task:taskRowToggleDetail", { label })}
        testId={`task-row-detail-toggle-${detail}`}
      />
    </div>
  );
}

function TaskRowDetailsSection({
  value,
  sort,
  onChange,
}: {
  value: SidebarTaskRowPresentation;
  sort: SortSpec;
  onChange: (value: SidebarTaskRowPresentation) => void;
}) {
  const { t } = useTranslation();
  const [announcement, setAnnouncement] = useState("");
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const detailOrder = useMemo(() => [...value.detailOrder], [value.detailOrder]);
  const relativeTimeDescriptionKey =
    sort.key === "lastActivityAt"
      ? "task:taskRowRelativeTimeLastActivity"
      : "task:taskRowRelativeTimeLastUpdate";

  function update(next: Partial<SidebarTaskRowPresentation>) {
    onChange({ ...value, ...next });
  }

  function handleDragEnd(event: DragEndEvent) {
    const activeId = String(event.active.id) as SidebarTaskRowDetail;
    const overId = event.over ? (String(event.over.id) as SidebarTaskRowDetail) : null;
    if (!overId) return;
    const nextOrder = reorderSidebarTaskRowDetails(detailOrder, activeId, overId);
    if (nextOrder === detailOrder) return;
    update({ detailOrder: nextOrder });
    setAnnouncement(
      t("task:taskRowReordered", {
        label: t(DETAIL_LABEL_KEYS[activeId]),
        position: nextOrder.indexOf(activeId) + 1,
      }),
    );
  }

  function handleToggle(detail: SidebarTaskRowDetail, checked: boolean) {
    const visibleDetails = checked
      ? [...new Set([...value.visibleDetails, detail])]
      : value.visibleDetails.filter((item) => item !== detail);
    update({ visibleDetails });
  }

  return (
    <div className="rounded-md border border-border/60 p-2">
      <div className="flex min-h-11 items-center gap-2">
        <div className="min-w-0 flex-1">
          <span className="block text-xs font-medium">{t("task:taskRowDetails")}</span>
          <span className="block text-[11px] text-muted-foreground">
            {t("task:taskRowDetailsDescription")}
          </span>
        </div>
        <TaskRowSwitch
          id="task-row-details-toggle"
          checked={value.detailsEnabled}
          onCheckedChange={(detailsEnabled) => update({ detailsEnabled })}
          ariaLabel={t("task:taskRowToggleDetails")}
          testId="task-row-details-toggle"
        />
      </div>
      {value.detailsEnabled && (
        <>
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext items={detailOrder} strategy={verticalListSortingStrategy}>
              <div className="mt-1">
                {detailOrder.map((detail) => (
                  <SortableDetailRow
                    key={detail}
                    detail={detail}
                    value={value}
                    onToggle={handleToggle}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
          <p className="px-1 pt-1 text-[11px] text-muted-foreground">
            {t(relativeTimeDescriptionKey)}
          </p>
        </>
      )}
      <div aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}

function TaskRowTrailingSelect({
  value,
  onChange,
}: {
  value: SidebarTaskRowPresentation;
  onChange: (value: SidebarTaskRowPresentation) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-11 items-center gap-2 rounded-md border border-border/60 p-2">
      <label htmlFor="task-row-trailing-select" className="min-w-0 flex-1 text-xs font-medium">
        {t("task:taskRowRightSide")}
      </label>
      <Select
        value={value.trailing}
        onValueChange={(trailing) =>
          onChange({ ...value, trailing: trailing as SidebarTaskRowTrailing })
        }
      >
        <SelectTrigger
          id="task-row-trailing-select"
          size="sm"
          className="min-h-11 w-[9rem] text-xs md:min-h-0 md:h-7"
          data-testid="task-row-trailing-select"
          aria-label={t("task:taskRowRightSide")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SIDEBAR_TASK_ROW_TRAILING_KEYS.map((trailing) => {
            const label = t(TRAILING_LABEL_KEYS[trailing]);
            return (
              <SelectItem
                key={trailing}
                value={trailing}
                className="text-xs"
                aria-label={label}
                description={t(TRAILING_DESCRIPTION_KEYS[trailing])}
              >
                {label}
              </SelectItem>
            );
          })}
        </SelectContent>
      </Select>
    </div>
  );
}

export function TaskRowSettings({
  value,
  sort,
  onChange,
}: {
  value: SidebarTaskRowPresentation;
  sort: SortSpec;
  onChange: (value: SidebarTaskRowPresentation) => void;
}) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const detailCountLabel = t("task:taskRowDetailsCount", {
    count: value.visibleDetails.length,
  });
  const trailingLabel = t(TRAILING_LABEL_KEYS[value.trailing]);

  return (
    <section className="border-t px-2 pb-2 pt-2" data-testid="task-row-settings">
      <Button
        type="button"
        variant="ghost"
        className="flex min-h-11 w-full items-center justify-between px-1 text-left"
        aria-expanded={expanded}
        aria-controls="task-row-settings-content"
        data-testid="task-row-settings-toggle"
        onClick={() => setExpanded((current) => !current)}
      >
        <span className="min-w-0">
          <span className="block text-[11px] font-medium uppercase leading-none tracking-wide text-muted-foreground">
            {t("task:taskRow")}
          </span>
          <span className="mt-1 block truncate text-xs text-muted-foreground">
            {value.detailsEnabled ? detailCountLabel : t("task:taskRowDetailsHidden")} {" · "}
            {trailingLabel}
          </span>
        </span>
        <IconChevronDown
          className={`size-4 shrink-0 text-muted-foreground transition-transform ${expanded ? "rotate-180" : ""}`}
          aria-hidden="true"
        />
      </Button>

      {expanded && (
        <div id="task-row-settings-content" className="space-y-2 pt-1">
          <TaskRowDetailsSection value={value} sort={sort} onChange={onChange} />
          <TaskRowTrailingSelect value={value} onChange={onChange} />
        </div>
      )}
    </section>
  );
}
