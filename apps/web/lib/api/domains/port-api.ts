import { getWebSocketClient } from "@/lib/ws/connection";

export type ListeningPort = { port: number; address: string };

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
