export function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
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
