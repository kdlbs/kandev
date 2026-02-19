"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { useTheme } from "next-themes";
import { getBackendConfig } from "@/lib/config";

interface AgentLogoProps {
  agentName: string;
  size?: number;
  className?: string;
}

const emptySubscribe = () => () => {};

// Module-level cache: logo URL → object URL (blob). Survives re-renders and remounts.
const logoCache = new Map<string, string>();

function fetchLogo(url: string): Promise<string> {
  const cached = logoCache.get(url);
  if (cached) return Promise.resolve(cached);

  return fetch(url)
    .then((res) => {
      if (!res.ok) throw new Error(`Logo fetch failed: ${res.status}`);
      return res.blob();
    })
    .then((blob) => {
      const objectUrl = URL.createObjectURL(blob);
      logoCache.set(url, objectUrl);
      return objectUrl;
    });
}

/** Wrapper that re-mounts the inner image when agentName changes, resetting error state. */
export function AgentLogo({ agentName, size = 16, className }: AgentLogoProps) {
  return <AgentLogoImage key={agentName} agentName={agentName} size={size} className={className} />;
}

function AgentLogoImage({ agentName, size = 16, className }: AgentLogoProps) {
  const { resolvedTheme } = useTheme();
  const mounted = useSyncExternalStore(
    emptySubscribe,
    () => true,
    () => false,
  );
  const [fetchedSrc, setFetchedSrc] = useState<string | null>(null);
  const [error, setError] = useState(false);

  const variant = resolvedTheme === "dark" ? "dark" : "light";
  const logoUrl = mounted
    ? `${getBackendConfig().apiBaseUrl}/api/v1/agents/${encodeURIComponent(agentName)}/logo?variant=${variant}`
    : null;

  // Read from cache during render to avoid setState in effect
  const cachedSrc = logoUrl ? (logoCache.get(logoUrl) ?? null) : null;

  useEffect(() => {
    if (!logoUrl || logoCache.has(logoUrl)) return;

    fetchLogo(logoUrl)
      .then(setFetchedSrc)
      .catch(() => setError(true));
  }, [logoUrl]);

  const src = cachedSrc ?? fetchedSrc;

  if (!mounted || error || !src) {
    return (
      <svg
        width={size}
        height={size}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        className={className}
        style={{ display: "inline-block", opacity: 0.5 }}
      >
        <polyline points="4 17 10 11 4 5" />
        <line x1="12" y1="19" x2="20" y2="19" />
      </svg>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={src} alt="" width={size} height={size} className={className} />
  );
}
