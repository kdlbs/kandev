import type { ListeningPort } from "@/lib/api/domains/port-api";

export type ForwardablePort = {
  port: number;
  address?: string;
  process?: string;
  badge: "detected" | "manual";
  tunnelPort?: number;
};

export function buildPortForwardRows(
  detectedPorts: readonly ListeningPort[],
  manualPorts: readonly number[],
  activeTunnels: ReadonlyMap<number, number>,
): ForwardablePort[] {
  const byPort = new Map<number, ForwardablePort>();
  for (const detected of detectedPorts) {
    if (!byPort.has(detected.port)) {
      byPort.set(detected.port, { ...detected, badge: "detected" });
    }
  }
  for (const port of [...manualPorts, ...activeTunnels.keys()]) {
    if (!byPort.has(port)) byPort.set(port, { port, badge: "manual" });
  }
  const rows = [...byPort.values()].map((row) => {
    const tunnelPort = activeTunnels.get(row.port);
    return tunnelPort === undefined ? row : { ...row, tunnelPort };
  });
  return rows.sort(
    (a, b) =>
      Number(b.tunnelPort !== undefined) - Number(a.tunnelPort !== undefined) || a.port - b.port,
  );
}
