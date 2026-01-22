import { useEffect } from 'react';
import { useAppStore } from '@/components/state-provider';
import { listBoards } from '@/lib/api';

export function useBoards(workspaceId: string | null, enabled = true) {
  const boards = useAppStore((state) => state.boards.items);
  const setBoards = useAppStore((state) => state.setBoards);

  useEffect(() => {
    if (!enabled || !workspaceId) return;
    listBoards(workspaceId, { cache: 'no-store' })
      .then((response) => {
        const mapped = response.boards.map((board) => ({
          id: board.id,
          workspaceId: board.workspace_id,
          name: board.name,
        }));
        setBoards(mapped);
      })
      .catch(() => setBoards([]));
  }, [enabled, setBoards, workspaceId]);

  return { boards };
}
