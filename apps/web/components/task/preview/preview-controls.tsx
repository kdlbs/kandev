'use client';

import { useMemo, useState } from 'react';
import {
  IconDeviceDesktop,
  IconLayoutSidebarLeftCollapse,
  IconLayoutSidebarRightCollapse,
} from '@tabler/icons-react';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { startProcess } from '@/lib/api';
import { useAppStore } from '@/components/state-provider';
import { useLayoutStore } from '@/lib/state/layout-store';

type PreviewControlsProps = {
  activeSessionId: string | null;
  hasDevScript: boolean;
};

export function PreviewControls({ activeSessionId, hasDevScript }: PreviewControlsProps) {
  const previewOpen = useAppStore((state) =>
    activeSessionId ? state.previewPanel.openBySessionId[activeSessionId] ?? false : false
  );
  const closeLayoutPreview = useLayoutStore((state) => state.closePreview);
  const applyPreset = useLayoutStore((state) => state.applyPreset);
  const showColumn = useLayoutStore((state) => state.showColumn);
  const toggleColumn = useLayoutStore((state) => state.toggleColumn);
  const toggleRightPanel = useLayoutStore((state) => state.toggleRightPanel);
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const layoutState = useMemo(() => {
    if (!activeSessionId) return null;
    return layoutBySession[activeSessionId] ?? {
      left: true,
      chat: true,
      right: true,
      preview: false,
    };
  }, [layoutBySession, activeSessionId]);
  const leftHidden = Boolean(layoutState && !layoutState.left);
  const rightHidden = Boolean(layoutState && !layoutState.right);
  const setPreviewOpen = useAppStore((state) => state.setPreviewOpen);
  const setPreviewStage = useAppStore((state) => state.setPreviewStage);
  const setPreviewView = useAppStore((state) => state.setPreviewView);
  const devProcessId = useAppStore((state) =>
    activeSessionId ? state.processes.devProcessBySessionId[activeSessionId] : undefined
  );
  const devProcess = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId] : undefined
  );
  const isDevRunning = devProcess?.status === 'running';
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);
  const [isStartingPreview, setIsStartingPreview] = useState(false);

  const handleTogglePreview = () => {
    if (!activeSessionId) return;
    const nextOpen = !previewOpen;
    if (nextOpen) {
      setPreviewOpen(activeSessionId, true);
      setPreviewView(activeSessionId, 'output');
      setPreviewStage(activeSessionId, 'logs');
      applyPreset(activeSessionId, 'preview');
      if (hasDevScript && !isStartingPreview) {
        const running = devProcess?.status === 'running' || devProcess?.status === 'starting';
        if (!running) {
          setIsStartingPreview(true);
          startProcess(activeSessionId, { kind: 'dev' })
            .then((resp) => {
              if (resp?.process) {
                const status = {
                  processId: resp.process.id,
                  sessionId: resp.process.session_id,
                  kind: resp.process.kind,
                  scriptName: resp.process.script_name,
                  status: resp.process.status,
                  command: resp.process.command,
                  workingDir: resp.process.working_dir,
                  exitCode: resp.process.exit_code ?? null,
                  startedAt: resp.process.started_at,
                  updatedAt: resp.process.updated_at,
                };
                upsertProcessStatus(status);
                setActiveProcess(status.sessionId, status.processId);
              }
            })
            .finally(() => {
              setIsStartingPreview(false);
            });
        }
      }
    } else {
      setPreviewOpen(activeSessionId, false);
      setPreviewStage(activeSessionId, 'closed');
      closeLayoutPreview(activeSessionId);
    }
  };

  if (!hasDevScript || !activeSessionId) {
    return null;
  }

  return (
    <div className="flex items-center gap-2">
      {previewOpen ? (
        <div className="inline-flex items-center rounded-md border border-border/70 bg-background">
          {leftHidden ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="cursor-pointer rounded-none border-r border-border/70"
                  onClick={() => {
                    showColumn(activeSessionId, 'left');
                  }}
                >
                  <IconLayoutSidebarLeftCollapse className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Show sidebar</TooltipContent>
            </Tooltip>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="cursor-pointer rounded-none border-r border-border/70"
                  onClick={() => {
                    toggleColumn(activeSessionId, 'left');
                  }}
                >
                  <IconLayoutSidebarLeftCollapse className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Hide sidebar</TooltipContent>
            </Tooltip>
          )}
          {rightHidden ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="cursor-pointer rounded-none"
                  onClick={() => {
                    toggleRightPanel(activeSessionId);
                  }}
                >
                  <IconLayoutSidebarRightCollapse className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Show right panel</TooltipContent>
            </Tooltip>
          ) : (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  className="cursor-pointer rounded-none"
                  onClick={() => {
                    toggleRightPanel(activeSessionId);
                  }}
                >
                  <IconLayoutSidebarRightCollapse className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Hide right panel</TooltipContent>
            </Tooltip>
          )}
        </div>
      ) : null}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            size="icon-sm"
            variant="outline"
            className="relative cursor-pointer border-border/70 bg-muted/30 hover:bg-muted/50"
            onClick={handleTogglePreview}
          >
            <IconDeviceDesktop className="h-3 w-3" />
            {isDevRunning ? (
              <span className="absolute -right-0.5 -top-0.5 h-2 w-2 rounded-full bg-emerald-500 ring-2 ring-background" />
            ) : null}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {previewOpen ? 'Hide Preview' : 'Show Preview'}
        </TooltipContent>
      </Tooltip>
    </div>
  );
}
