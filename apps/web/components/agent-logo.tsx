'use client';

import { useState, useSyncExternalStore } from 'react';
import Image from 'next/image';
import { useTheme } from 'next-themes';
import { getBackendConfig } from '@/lib/config';

interface AgentLogoProps {
  agentName: string;
  size?: number;
  className?: string;
}

const emptySubscribe = () => () => {};

/** Wrapper that re-mounts the inner image when agentName changes, resetting error state. */
export function AgentLogo({ agentName, size = 16, className }: AgentLogoProps) {
  return <AgentLogoImage key={agentName} agentName={agentName} size={size} className={className} />;
}

function AgentLogoImage({ agentName, size = 16, className }: AgentLogoProps) {
  const { resolvedTheme } = useTheme();
  const mounted = useSyncExternalStore(emptySubscribe, () => true, () => false);
  const [error, setError] = useState(false);

  if (!mounted || error) {
    return <span style={{ display: 'inline-block', width: size, height: size }} className={className} />;
  }

  const variant = resolvedTheme === 'dark' ? 'dark' : 'light';
  const src = `${getBackendConfig().apiBaseUrl}/api/v1/agents/${encodeURIComponent(agentName)}/logo?variant=${variant}`;

  return (
    <Image
      src={src}
      alt=""
      width={size}
      height={size}
      className={className}
      unoptimized
      onError={() => setError(true)}
    />
  );
}
