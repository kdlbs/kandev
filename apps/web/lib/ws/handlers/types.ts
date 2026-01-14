import type { BackendMessageMap } from '@/lib/types/backend';

export type WsHandlers = {
  [K in keyof BackendMessageMap]?: (message: BackendMessageMap[K]) => void;
};
