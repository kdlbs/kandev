"use client";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
  type PointerEvent,
  type KeyboardEvent,
} from "react";
import { SidebarCustomizationFeedback } from "./sidebar-customization-feedback";
import { useTranslation } from "react-i18next";
import { IconChevronDown, IconChevronUp } from "@tabler/icons-react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAppStore } from "@/components/state-provider";
import { useSidebarCustomization } from "@/hooks/domains/sidebar/use-sidebar-customization";
import { navigationGeometry, savedNavigationHeight } from "@/lib/sidebar/navigation-geometry";

function useSplitMeasurements() {
  const root = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [size, setSize] = useState({ available: 0, content: 0 });
  useLayoutEffect(() => {
    const measure = () =>
      setSize({
        available: root.current?.clientHeight ?? 0,
        content: content.current?.scrollHeight ?? 0,
      });
    const observer = new ResizeObserver(measure);
    if (root.current) observer.observe(root.current);
    if (content.current) observer.observe(content.current);
    measure();
    return () => observer.disconnect();
  }, []);
  return { root, content, size };
}

function useSplitDrag(height: number, maximum: number, commit: (height: number) => void) {
  const [preview, setPreview] = useState<number | undefined>();
  const drag = useRef<{ y: number; height: number; cursor: string } | null>(null);
  const restore = () => {
    if (drag.current) document.documentElement.style.cursor = drag.current.cursor;
    drag.current = null;
    setPreview(undefined);
  };
  useEffect(
    () => () => {
      if (drag.current) document.documentElement.style.cursor = drag.current.cursor;
    },
    [],
  );
  const onPointerDown = (event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || !event.isPrimary || (event.target as Element).closest("button"))
      return;
    event.preventDefault();
    event.currentTarget.focus();
    event.currentTarget.setPointerCapture(event.pointerId);
    drag.current = { y: event.clientY, height, cursor: document.documentElement.style.cursor };
    document.documentElement.style.cursor = "row-resize";
    setPreview(height);
  };
  const onPointerMove = (event: PointerEvent<HTMLDivElement>) => {
    const current = drag.current;
    if (current)
      setPreview(Math.max(0, Math.min(maximum, current.height + event.clientY - current.y)));
  };
  const onPointerUp = (event: PointerEvent<HTMLDivElement>) => {
    if (!drag.current) return;
    const next = Math.max(
      0,
      Math.min(maximum, drag.current.height + event.clientY - drag.current.y),
    );
    const start = Math.max(0, Math.min(maximum, drag.current.height));
    restore();
    event.currentTarget.releasePointerCapture(event.pointerId);
    if (Math.round(next) !== Math.round(start)) commit(next);
  };
  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      restore();
      return;
    }
    if (!["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
    event.preventDefault();
    let next = height + (event.key === "ArrowUp" ? -16 : 16);
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = maximum;
    commit(Math.max(0, Math.min(maximum, next)));
  };
  return {
    preview,
    handlers: {
      onPointerDown,
      onPointerMove,
      onPointerUp,
      onPointerCancel: restore,
      onLostPointerCapture: restore,
      onKeyDown,
    },
  };
}

export function SidebarNavigationSplit({
  navigation,
  tasks,
}: {
  navigation: ReactNode;
  tasks: ReactNode;
}) {
  const { t } = useTranslation();
  const { root, content, size } = useSplitMeasurements();
  const { isFinePointer } = useResponsiveBreakpoint();
  const dividerHeight = isFinePointer ? 12 : 44;
  const maximum = Math.max(0, size.available - 112 - dividerHeight);
  const clip = useRef<HTMLDivElement>(null);
  const layout = useAppStore(
    (s) => s.userSettings.sidebarLayoutsByWorkspace[s.workspaces.activeId ?? ""],
  );
  const { mutate, status } = useSidebarCustomization();
  const savedHeight = layout?.navigation_height;
  const geometry = navigationGeometry(
    size.available,
    size.content,
    savedHeight,
    layout?.navigation_expanded ?? false,
    dividerHeight,
  );
  const commit = (height: number) =>
    void mutate((current) => ({
      ...current,
      navigationHeight: savedNavigationHeight(height),
      navigationExpanded: false,
    }));
  const { preview, handlers } = useSplitDrag(geometry.height, Math.max(0, maximum), commit);
  const rendered =
    preview === undefined
      ? geometry
      : navigationGeometry(size.available, size.content, preview, false, dividerHeight);
  const isExpanded = Boolean(layout?.navigation_expanded) && preview === undefined;
  const toggle = () =>
    void mutate((current) => ({
      ...current,
      navigationHeight: current.navigationHeight ?? savedNavigationHeight(rendered.height),
      navigationExpanded: !current.navigationExpanded,
    }));
  useClippedNavigationFocus(content, clip, rendered.height, isExpanded, navigation);
  return (
    <div ref={root} className="flex min-h-0 flex-1 flex-col" data-testid="sidebar-navigation-split">
      <div
        ref={clip}
        id="sidebar-navigation-content"
        className="relative shrink-0"
        style={{ height: rendered.height, overflowY: isExpanded ? "auto" : "hidden" }}
        onFocusCapture={(event) => {
          if (
            !isExpanded &&
            clip.current &&
            (event.target as HTMLElement).getBoundingClientRect().bottom >
              clip.current.getBoundingClientRect().bottom
          )
            toggle();
        }}
      >
        <div ref={content}>{navigation}</div>
        {rendered.clipped && !isExpanded && (
          <div
            aria-hidden="true"
            data-testid="sidebar-navigation-fade"
            className="pointer-events-none absolute inset-x-0 bottom-0 h-6 bg-gradient-to-t from-background to-transparent"
          />
        )}
      </div>
      <div
        role="separator"
        aria-orientation="horizontal"
        aria-label={t("settings:sidebarResizeNavigation")}
        aria-valuemin={Math.min(0, maximum, size.content)}
        aria-valuemax={maximum}
        aria-valuenow={Math.round(rendered.height)}
        tabIndex={0}
        data-testid="sidebar-navigation-divider"
        {...handlers}
        className="relative z-10 flex h-3 shrink-0 touch-none items-center justify-center border-b border-border/60 outline-none focus-visible:ring-2 focus-visible:ring-ring cursor-row-resize [@media(pointer:coarse)]:h-11"
      >
        <SplitDisclosure
          visible={rendered.clipped || (savedHeight !== undefined && geometry.expandable)}
          expanded={isExpanded}
          disabled={status === "saving"}
          onClick={toggle}
        />
      </div>
      <div className="flex min-h-0 flex-1 flex-col [&>div]:border-t-0 [&>div]:pt-1">{tasks}</div>
      <SidebarCustomizationFeedback status={status} />
    </div>
  );
}

function SplitDisclosure({
  visible,
  expanded,
  disabled,
  onClick,
}: {
  visible: boolean;
  expanded: boolean;
  disabled: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  if (!visible) return null;
  return (
    <button
      type="button"
      disabled={disabled}
      aria-label={t(
        expanded ? "settings:sidebarCollapseNavigation" : "settings:sidebarExpandNavigation",
      )}
      aria-expanded={expanded}
      aria-controls="sidebar-navigation-content"
      onClick={onClick}
      data-testid="sidebar-navigation-expand"
      className="absolute left-1/2 -translate-x-1/2 -top-1.5 z-10 flex h-6 w-8 items-center justify-center rounded text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring [@media(pointer:coarse)]:top-0 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
    >
      {expanded ? <IconChevronUp className="size-3" /> : <IconChevronDown className="size-3" />}
    </button>
  );
}

function useClippedNavigationFocus(
  content: RefObject<HTMLDivElement | null>,
  clip: RefObject<HTMLDivElement | null>,
  height: number,
  isExpanded: boolean,
  navigation: ReactNode,
) {
  useLayoutEffect(() => {
    if (!content.current || !clip.current) return;
    const bottom = clip.current.getBoundingClientRect().bottom;
    const rows = content.current.querySelectorAll<HTMLElement>("[data-sidebar-node-id]");
    rows.forEach((row) => {
      row.inert = !isExpanded && row.getBoundingClientRect().top >= bottom;
    });
    return () =>
      rows.forEach((row) => {
        row.inert = false;
      });
  }, [content, clip, height, isExpanded, navigation]);
}
