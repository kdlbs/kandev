"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLayoutGrid, IconListDetails } from "@tabler/icons-react";
import Link from "@/components/routing/app-link";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { usePathname } from "@/lib/routing/client-router";
import {
  canvasHref,
  listWorkspaceCanvases,
  workspaceCanvasSettingsHref,
  type Canvas,
} from "@/lib/api/domains/canvas-api";
import { cn } from "@/lib/utils";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";
import {
  APP_SIDEBAR_SECTION_IDS,
  SIDEBAR_ITEM_ACTIVE,
  SIDEBAR_ITEM_INACTIVE,
} from "../app-sidebar-constants";
import { AppSidebarSection } from "../app-sidebar-section";

const EMPTY_CANVASES: Canvas[] = [];

export function isActiveWorkspaceCanvas(canvas: Canvas): boolean {
  return (
    canvas.scope_kind === "workspace" &&
    canvas.status === "active" &&
    canvas.active_release_status === "valid"
  );
}

export function useWorkspaceCanvases(
  workspaceId: string | null,
  includeArchived = false,
): Canvas[] {
  const [loaded, setLoaded] = useState<{ workspaceId: string | null; canvases: Canvas[] }>({
    workspaceId: null,
    canvases: EMPTY_CANVASES,
  });
  const requestRef = useRef(0);
  const lifecycleRevision = useCanvasLifecycleRevision();

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!workspaceId) {
      setLoaded({ workspaceId: null, canvases: EMPTY_CANVASES });
      return;
    }

    setLoaded((current) => ({
      workspaceId,
      canvases: current.workspaceId === workspaceId ? current.canvases : EMPTY_CANVASES,
    }));
    listWorkspaceCanvases(workspaceId, { includeArchived })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ workspaceId, canvases: response?.canvases ?? EMPTY_CANVASES });
      })
      .catch(() => {
        if (requestRef.current === requestId) setLoaded({ workspaceId, canvases: EMPTY_CANVASES });
      });
  }, [includeArchived, lifecycleRevision, workspaceId]);

  return loaded.workspaceId === workspaceId ? loaded.canvases : EMPTY_CANVASES;
}

function OpenCanvasSettingsShortcut({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Link
          href={workspaceCanvasSettingsHref(workspaceId)}
          aria-label={t("canvases:openWorkspaceSettings")}
          data-testid="sidebar-canvases-settings"
          className="flex h-5 w-5 items-center justify-center rounded text-muted-foreground/70 transition-colors hover:bg-muted/60 hover:text-foreground cursor-pointer"
        >
          <IconListDetails className="h-3.5 w-3.5" />
        </Link>
      </TooltipTrigger>
      <TooltipContent side="right">{t("canvases:openWorkspaceSettings")}</TooltipContent>
    </Tooltip>
  );
}

function CanvasRow({ canvas, active }: { canvas: Canvas; active: boolean }) {
  return (
    <Link
      href={canvasHref(canvas.id)}
      data-testid={`sidebar-canvas-${canvas.id}`}
      className={cn(
        "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-[13px] font-medium cursor-pointer",
        active ? SIDEBAR_ITEM_ACTIVE : SIDEBAR_ITEM_INACTIVE,
      )}
    >
      <IconLayoutGrid className="h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate">{canvas.title}</span>
    </Link>
  );
}

function EmptyCanvasRow({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  return (
    <Link
      href={workspaceCanvasSettingsHref(workspaceId)}
      data-testid="sidebar-canvases-empty"
      className="rounded-md px-2.5 py-1.5 text-[13px] text-muted-foreground hover:bg-muted/60 hover:text-foreground cursor-pointer"
    >
      {t("canvases:setUpCanvas")}
    </Link>
  );
}

export function CanvasesSection({ collapsed }: { collapsed: boolean }) {
  const { t } = useTranslation();
  const pathname = usePathname();
  const enabled = useFeature("canvases");
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);
  const workspaceId = enabled ? activeWorkspaceId : null;
  const canvases = useWorkspaceCanvases(workspaceId);
  const activeCanvases = canvases.filter(isActiveWorkspaceCanvas);

  if (!enabled || !activeWorkspaceId) return null;

  return (
    <AppSidebarSection
      id={APP_SIDEBAR_SECTION_IDS.canvases}
      label={t("canvases:canvases")}
      collapsed={collapsed}
      icon={IconLayoutGrid}
      headerAction={<OpenCanvasSettingsShortcut workspaceId={activeWorkspaceId} />}
      headerActionVisibility="always"
      collapsedSummary={activeCanvases.length > 0 ? activeCanvases.length : undefined}
      defaultExpanded={false}
    >
      {activeCanvases.length === 0 ? (
        <EmptyCanvasRow workspaceId={activeWorkspaceId} />
      ) : (
        activeCanvases.map((canvas) => (
          <CanvasRow key={canvas.id} canvas={canvas} active={pathname === canvasHref(canvas.id)} />
        ))
      )}
    </AppSidebarSection>
  );
}
