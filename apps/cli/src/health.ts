export function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Resolves the health check timeout, allowing override via environment variable.
 *
 * The KANDEV_HEALTH_TIMEOUT_MS environment variable can override the default
 * timeout for waiting on backend health checks. This is useful for slower
 * machines or debugging scenarios where the backend takes longer to start.
 *
 * @param defaultMs - Default timeout in milliseconds if env var is not set
 * @returns The resolved timeout in milliseconds
 */
export function resolveHealthTimeoutMs(defaultMs: number): number {
  const raw = process.env.KANDEV_HEALTH_TIMEOUT_MS;
  if (!raw) {
    return defaultMs;
  }
  const parsed = Number(raw);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return defaultMs;
  }
  return Math.floor(parsed);
}

export async function waitForHealth(
  baseUrl: string,
  proc: { exitCode: number | null },
  timeoutMs: number,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  const healthUrl = `${baseUrl}/health`;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) {
      throw new Error("Backend exited before healthcheck passed");
    }
    try {
      const res = await fetch(healthUrl);
      if (res.ok) {
        return;
      }
    } catch {
      // ignore until timeout
    }
    await delay(300);
  }
  throw new Error(`Backend healthcheck timed out after ${timeoutMs}ms`);
}

export async function waitForUrlReady(
  url: string,
  proc: { exitCode: number | null },
  timeoutMs: number,
): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) {
      throw new Error("Web process exited before URL became reachable");
    }
    try {
      await fetch(url);
      return;
    } catch {
      // ignore until timeout
    }
    await delay(300);
  }
  throw new Error(`Web URL readiness timed out after ${timeoutMs}ms (${url})`);
}
