"use client";

import { useMemo, useState } from "react";
import {
  DndContext,
  KeyboardSensor,
  MouseSensor,
  TouchSensor,
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
import { IconGripVertical } from "@tabler/icons-react";
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
import { SidebarSettingsDisclosure } from "./sidebar-settings-disclosure";
import { sidebarSortHasKey } from "@/lib/sidebar/sidebar-sort-chain";
import { SidebarReorderMenu } from "./sidebar-reorder-menu";
import { createSidebarListCollisionDetection } from "./sidebar-reorder-collision";

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

function taskRowDetailLabel(id: string, t: ReturnType<typeof useTranslation>["t"]) {
  return t(DETAIL_LABEL_KEYS[id as SidebarTaskRowDetail] ?? "task:taskRowDetails");
}

function taskRowRelativeTimeDescriptionKey(sort: SortSpec) {
  return sidebarSortHasKey(sort, "lastActivityAt")
    ? "task:taskRowRelativeTimeLastActivity"
    : "task:taskRowRelativeTimeLastUpdate";
}

function TaskRowSwitch({
  id,
  checked,
  onCheckedChange,
  ariaLabel,
  testId,
  isDrawerLayout,
}: {
  id: string;
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  ariaLabel: string;
  testId: string;
  isDrawerLayout: boolean;
}) {
  const targetClass = isDrawerLayout
    ? "size-11"
    : "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11";
  return (
    <label
      htmlFor={id}
      className={`flex shrink-0 cursor-pointer items-center justify-center ${targetClass}`}
      data-testid={testId}
    >
      <Switch
        id={id}
        size="sm"
        checked={checked}
        onCheckedChange={onCheckedChange}
        aria-label={ariaLabel}
        className={
          isDrawerLayout
            ? "after:-inset-y-4"
            : "max-md:after:-inset-y-4 [@media(pointer:coarse)]:after:-inset-y-4"
        }
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
  isDrawerLayout,
  onToggle,
  onMove,
}: {
  detail: SidebarTaskRowDetail;
  value: SidebarTaskRowPresentation;
  isDrawerLayout: boolean;
  onToggle: (detail: SidebarTaskRowDetail, checked: boolean) => void;
  onMove: (offset: -1 | 1) => void;
}) {
  const { t } = useTranslation();
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: detail });
  const label = t(DETAIL_LABEL_KEYS[detail]);
  const handleClass = isDrawerLayout
    ? "size-11"
    : "size-7 max-md:size-11 [@media(pointer:coarse)]:size-11";

  return (
    <div
      ref={setNodeRef}
      data-testid={`task-row-detail-${detail}`}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className="flex min-h-11 items-center gap-1 rounded-md border-b border-border/40 last:border-b-0"
      data-dragging={isDragging ? "true" : undefined}
    >
      <button
        ref={setActivatorNodeRef}
        type="button"
        aria-label={t("task:taskRowReorderHandle", { label })}
        className={`flex shrink-0 touch-none items-center justify-center text-muted-foreground/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${handleClass}`}
        data-testid={`task-row-detail-handle-${detail}`}
        data-vaul-no-drag={isDrawerLayout ? "" : undefined}
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
      <SidebarReorderMenu
        label={label}
        position={value.detailOrder.indexOf(detail) + 1}
        count={value.detailOrder.length}
        onMove={onMove}
        isDrawerLayout={isDrawerLayout}
        testId={`task-row-detail-more-${detail}`}
      />
      <TaskRowSwitch
        id={`task-row-detail-${detail}-toggle`}
        checked={value.visibleDetails.includes(detail)}
        onCheckedChange={(checked) => onToggle(detail, checked)}
        ariaLabel={t("task:taskRowToggleDetail", { label })}
        testId={`task-row-detail-toggle-${detail}`}
        isDrawerLayout={isDrawerLayout}
      />
    </div>
  );
}

function TaskRowDetailList({
  value,
  detailOrder,
  isDrawerLayout,
  reorderScopeKey,
  relativeTimeDescriptionKey,
  sensors,
  announcement,
  detailLabel,
  onToggle,
  onMove,
  onDragEnd,
}: {
  value: SidebarTaskRowPresentation;
  detailOrder: SidebarTaskRowDetail[];
  isDrawerLayout: boolean;
  reorderScopeKey: string;
  relativeTimeDescriptionKey: string;
  sensors: ReturnType<typeof useSensors>;
  announcement: string;
  detailLabel: (id: string) => string;
  onToggle: (detail: SidebarTaskRowDetail, checked: boolean) => void;
  onMove: (detail: SidebarTaskRowDetail, offset: -1 | 1) => void;
  onDragEnd: (event: DragEndEvent) => void;
}) {
  const { t } = useTranslation();
  const collision = useMemo(createSidebarListCollisionDetection, []);

  return (
    <>
      <DndContext
        key={reorderScopeKey}
        sensors={sensors}
        collisionDetection={collision.detect}
        onDragStart={collision.reset}
        onDragEnd={(event) => {
          const dropEvent = collision.isDropWithinCurrentVisibleBounds()
            ? event
            : { ...event, over: null };
          onDragEnd(dropEvent);
        }}
        onDragCancel={collision.reset}
        accessibility={{
          screenReaderInstructions: { draggable: t("task:sidebarReorderInstructions") },
          announcements: {
            onDragStart: ({ active }) => {
              const position = detailOrder.indexOf(String(active.id) as SidebarTaskRowDetail) + 1;
              return t("task:sidebarReorderPickedUp", {
                label: detailLabel(String(active.id)),
                position,
                count: detailOrder.length,
              });
            },
            onDragOver: ({ active, over }) => {
              if (!over) return;
              return t("task:sidebarReorderMoved", {
                label: detailLabel(String(active.id)),
                position: detailOrder.indexOf(String(over.id) as SidebarTaskRowDetail) + 1,
                count: detailOrder.length,
              });
            },
            onDragEnd: ({ active, over }) => {
              if (!over || !collision.isDropWithinCurrentVisibleBounds()) {
                return t("task:sidebarReorderCancelled", {
                  label: detailLabel(String(active.id)),
                });
              }
              return t("task:sidebarReorderDropped", {
                label: detailLabel(String(active.id)),
                position: detailOrder.indexOf(String(over.id) as SidebarTaskRowDetail) + 1,
              });
            },
            onDragCancel: ({ active }) =>
              t("task:sidebarReorderCancelled", { label: detailLabel(String(active.id)) }),
          },
        }}
      >
        <SortableContext items={detailOrder} strategy={verticalListSortingStrategy}>
          <div className="mt-1" data-sidebar-reorder-list="">
            {detailOrder.map((detail) => (
              <SortableDetailRow
                key={detail}
                detail={detail}
                value={value}
                isDrawerLayout={isDrawerLayout}
                onToggle={onToggle}
                onMove={(offset) => onMove(detail, offset)}
              />
            ))}
          </div>
        </SortableContext>
      </DndContext>
      <p className="px-1 pt-1 text-[11px] text-muted-foreground">{t(relativeTimeDescriptionKey)}</p>
      <div aria-live="polite" className="sr-only">
        {announcement}
      </div>
    </>
  );
}

function TaskRowDetailsSection({
  value,
  sort,
  isDrawerLayout,
  reorderScopeKey,
  onChange,
}: {
  value: SidebarTaskRowPresentation;
  sort: SortSpec;
  isDrawerLayout: boolean;
  reorderScopeKey: string;
  onChange: (value: SidebarTaskRowPresentation) => void;
}) {
  const { t } = useTranslation();
  const [announcement, setAnnouncement] = useState("");
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 8 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const detailOrder = value.detailOrder;
  const relativeTimeDescriptionKey = taskRowRelativeTimeDescriptionKey(sort);

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

  function moveDetail(detail: SidebarTaskRowDetail, offset: -1 | 1) {
    const index = detailOrder.indexOf(detail);
    const destination = index + offset;
    if (index < 0 || destination < 0 || destination >= detailOrder.length) return;
    const target = detailOrder[destination];
    if (!target) return;
    const nextOrder = reorderSidebarTaskRowDetails(detailOrder, detail, target);
    if (nextOrder === detailOrder) return;
    update({ detailOrder: nextOrder });
    setAnnouncement(
      t("task:sidebarReorderMoved", {
        label: t(DETAIL_LABEL_KEYS[detail]),
        position: destination + 1,
        count: detailOrder.length,
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
          isDrawerLayout={isDrawerLayout}
        />
      </div>
      {value.detailsEnabled && (
        <TaskRowDetailList
          value={value}
          detailOrder={detailOrder}
          isDrawerLayout={isDrawerLayout}
          reorderScopeKey={reorderScopeKey}
          relativeTimeDescriptionKey={relativeTimeDescriptionKey}
          sensors={sensors}
          announcement={announcement}
          detailLabel={(id) => taskRowDetailLabel(id, t)}
          onToggle={handleToggle}
          onMove={moveDetail}
          onDragEnd={handleDragEnd}
        />
      )}
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
  isDrawerLayout = false,
  reorderScopeKey = "default",
  onChange,
}: {
  value: SidebarTaskRowPresentation;
  sort: SortSpec;
  isDrawerLayout?: boolean;
  reorderScopeKey?: string;
  onChange: (value: SidebarTaskRowPresentation) => void;
}) {
  const { t } = useTranslation();
  const detailCountLabel = t("task:taskRowDetailsCount", {
    count: value.visibleDetails.length,
  });
  const trailingLabel = t(TRAILING_LABEL_KEYS[value.trailing]);

  return (
    <SidebarSettingsDisclosure
      title={t("task:taskRow")}
      summary={
        <span className="flex min-w-0 items-center gap-1">
          <span className="truncate">
            {value.detailsEnabled ? detailCountLabel : t("task:taskRowDetailsHidden")}
          </span>
          <span className="truncate">{trailingLabel}</span>
        </span>
      }
      testId="task-row-settings"
      className="border-b"
      contentClassName="space-y-2 pt-1"
    >
      <TaskRowDetailsSection
        value={value}
        sort={sort}
        isDrawerLayout={isDrawerLayout}
        reorderScopeKey={reorderScopeKey}
        onChange={onChange}
      />
      <TaskRowTrailingSelect value={value} onChange={onChange} />
    </SidebarSettingsDisclosure>
  );
}
