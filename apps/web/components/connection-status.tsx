'use client';

import { Badge } from '@kandev/ui/badge';
import { IconCircleFilled } from '@tabler/icons-react';
import { useAppStore } from '@/components/state-provider';

export function ConnectionStatus() {
  const status = useAppStore((store) => store.connection.status);
  const error = useAppStore((store) => store.connection.error);

  const getStatusConfig = () => {
    switch (status) {
      case 'connected':
        return { color: 'text-emerald-500', text: 'Connected', variant: 'default' as const, pulse: false };
      case 'connecting':
        return { color: 'text-amber-500', text: 'Connecting', variant: 'secondary' as const, pulse: true };
      case 'reconnecting':
        return { color: 'text-orange-500', text: 'Reconnecting', variant: 'secondary' as const, pulse: true };
      case 'error':
        return { color: 'text-red-500', text: 'Error', variant: 'destructive' as const, pulse: false };
      case 'disconnected':
      default:
        return { color: 'text-yellow-500', text: 'Disconnected', variant: 'secondary' as const, pulse: true };
    }
  };

  const config = getStatusConfig();

  const getTooltipContent = () => {
    const errorLine = error ? `\nError: ${error}` : '';
    return `Status: ${config.text}${errorLine}`;
  };

  return (
    <Badge
      variant={config.variant}
      title={getTooltipContent()}
      className={`flex items-center gap-1.5 px-2 py-1 text-xs cursor-default ${
        config.pulse ? 'animate-pulse' : ''
      }`}
    >
      <IconCircleFilled className={`h-2 w-2 ${config.color}`} />
      <span>{config.text}</span>
    </Badge>
  );
}
