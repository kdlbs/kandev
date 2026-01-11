'use client';

import { Badge } from '@/components/ui/badge';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { IconCircleFilled } from '@tabler/icons-react';
import { getLocalStorage } from '@/lib/local-storage';
import { STORAGE_KEYS, DEFAULT_BACKEND_URL } from '@/lib/settings/constants';

export function ConnectionStatus() {
  // Dummy status - will be replaced with Zustand store later
  const state = 'disconnected';
  const backendUrl = getLocalStorage(STORAGE_KEYS.BACKEND_URL, DEFAULT_BACKEND_URL);

  const getStatusConfig = () => {
    // For now, always show disconnected status
    return {
      color: 'bg-gray-500',
      text: 'Disconnected',
      variant: 'secondary' as const,
      pulse: false,
    };
  };

  const config = getStatusConfig();

  const getTooltipContent = () => {
    return `Status: ${config.text}\nBackend URL: ${backendUrl}\n\nWebSocket connection will be implemented with Zustand`;
  };

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant={config.variant} className="flex items-center gap-1.5 px-2 py-1 text-xs">
            <IconCircleFilled
              className={`h-2 w-2 ${config.color} ${config.pulse ? 'animate-pulse' : ''}`}
            />
            <span>{config.text}</span>
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          <p className="whitespace-pre-line text-xs">{getTooltipContent()}</p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
