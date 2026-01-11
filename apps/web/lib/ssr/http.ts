import type { Board, BoardSnapshot, Task } from '@/lib/types/http';

const API_BASE_URL = process.env.KANDEV_API_BASE_URL ?? 'http://localhost:8080';

async function fetchJson<T>(url: string): Promise<T> {
  const response = await fetch(url, { cache: 'no-store' });
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status} ${response.statusText}`);
  }
  return (await response.json()) as T;
}

export async function fetchBoards(): Promise<Board[]> {
  return fetchJson<Board[]>(`${API_BASE_URL}/api/v1/boards`);
}

export async function fetchBoardSnapshot(boardId: string): Promise<BoardSnapshot> {
  return fetchJson<BoardSnapshot>(`${API_BASE_URL}/api/v1/boards/${boardId}/snapshot`);
}

export async function fetchTask(taskId: string): Promise<Task> {
  return fetchJson<Task>(`${API_BASE_URL}/api/v1/tasks/${taskId}`);
}
