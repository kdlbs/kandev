'use client';

import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import {
  IconLoader2,
  IconPlugConnected,
  IconPlugOff,
  IconAlertTriangle,
} from '@tabler/icons-react';
import type { LspStatus } from '@/lib/lsp/lsp-client-manager';

type StaticConfig = { tooltip: string; clickable: boolean };

const ICON_CLS = 'h-3.5 w-3.5';

const ICONS: Record<string, React.ReactNode> = {
  disabled: <IconPlugOff className={`${ICON_CLS} text-muted-foreground/50`} />,
  connecting: <IconLoader2 className={`${ICON_CLS} animate-spin text-muted-foreground`} />,
  installing: <IconLoader2 className={`${ICON_CLS} animate-spin text-amber-500`} />,
  starting: <IconLoader2 className={`${ICON_CLS} animate-spin text-blue-500`} />,
  ready: <IconPlugConnected className={`${ICON_CLS} text-emerald-500`} />,
  stopping: <IconLoader2 className={`${ICON_CLS} animate-spin text-muted-foreground`} />,
  unavailable: <IconPlugOff className={`${ICON_CLS} text-muted-foreground`} />,
  error: <IconAlertTriangle className={`${ICON_CLS} text-yellow-500`} />,
};

const STATIC_CONFIGS: Record<string, StaticConfig> = {
  disabled: { tooltip: 'LSP: Off \u2014 click to start', clickable: true },
  connecting: { tooltip: 'LSP: Connecting...', clickable: true },
  installing: { tooltip: 'LSP: Installing language server...', clickable: false },
  starting: { tooltip: 'LSP: Starting language server...', clickable: true },
  ready: { tooltip: 'LSP: Connected \u2014 click to stop', clickable: true },
  stopping: { tooltip: 'LSP: Stopping...', clickable: false },
};

function getConfig(status: LspStatus): { icon: React.ReactNode; tooltip: string; clickable: boolean } | null {
  const icon = ICONS[status.state];
  if (!icon) return null;

  const sc = STATIC_CONFIGS[status.state];
  if (sc) return { icon, ...sc };

  // Dynamic tooltip for unavailable/error states
  const reason = 'reason' in status ? status.reason : null;
  if (status.state === 'unavailable') return { icon, tooltip: `LSP: ${reason ?? 'Unavailable'}`, clickable: true };
  if (status.state === 'error') return { icon, tooltip: `LSP: ${reason ?? 'Error'}`, clickable: true };

  return null;
}

export function LspStatusButton({
  status,
  lspLanguage,
  onToggle,
}: {
  status: LspStatus;
  lspLanguage: string | null;
  onToggle: () => void;
}) {
  if (!lspLanguage) return null;

  const c = getConfig(status);
  if (!c) return null;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={c.clickable ? onToggle : undefined}
          disabled={!c.clickable}
        >
          {c.icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{c.tooltip}</TooltipContent>
    </Tooltip>
  );
}
