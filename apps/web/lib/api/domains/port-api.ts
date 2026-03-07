import { getWebSocketClient } from "@/lib/ws/connection";

export type ListeningPort = { port: number; address: string; process?: string };

export type TunnelInfo = { port: number; tunnel_port: number };

export async function listPorts(sessionId: string): Promise<ListeningPort[]> {
  const client = getWebSocketClient();
  if (!client) return [];

  try {
    const response = (await client.request("port.list", {
      session_id: sessionId,
    })) as { ports: ListeningPort[] };
    return response.ports ?? [];
  } catch {
    return [];
  }
}

export async function startTunnel(
  sessionId: string,
  port: number,
  tunnelPort?: number,
): Promise<number> {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket not connected");

  const response = (await client.request("port.tunnel.start", {
    session_id: sessionId,
    port,
    tunnel_port: tunnelPort ?? 0,
  })) as { tunnel_port: number };
  return response.tunnel_port;
}

export async function stopTunnel(sessionId: string, port: number): Promise<void> {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket not connected");

  await client.request("port.tunnel.stop", {
    session_id: sessionId,
    port,
  });
}

export async function listTunnels(sessionId: string): Promise<TunnelInfo[]> {
  const client = getWebSocketClient();
  if (!client) return [];

  try {
    const response = (await client.request("port.tunnel.list", {
      session_id: sessionId,
    })) as { tunnels: TunnelInfo[] };
    return response.tunnels ?? [];
  } catch {
    return [];
  }
}
