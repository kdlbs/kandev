import type { BackendMessageMap, BackendMessageType } from '@/lib/types/backend';
import { generateUUID } from '@/lib/utils';

type MessageHandler<T extends BackendMessageType> = (message: BackendMessageMap[T]) => void;

type WebSocketStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error' | 'reconnecting';

export interface ReconnectOptions {
  enabled?: boolean;
  maxAttempts?: number;
  initialDelay?: number;
  maxDelay?: number;
  backoffMultiplier?: number;
}

export interface ConnectionMetrics {
  reconnectAttempts: number;
  lastDisconnectTime: number | null;
  lastDisconnectReason: string | null;
  totalReconnects: number;
  isHealthy: boolean;
}

const DEFAULT_RECONNECT_OPTIONS: Required<ReconnectOptions> = {
  enabled: true,
  maxAttempts: 10,
  initialDelay: 1000,
  maxDelay: 30000,
  backoffMultiplier: 1.5,
};

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private status: WebSocketStatus = 'idle';
  private handlers = new Map<BackendMessageType, Set<MessageHandler<BackendMessageType>>>();
  private pendingRequests = new Map<
    string,
    { resolve: (payload: unknown) => void; reject: (error: Error) => void; timeout: ReturnType<typeof setTimeout> }
  >();
  private pendingQueue: string[] = [];
  private reconnectOptions: Required<ReconnectOptions>;
  private reconnectAttempts = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private intentionalClose = false;
  private subscriptions = new Map<string, number>();
  private userSubscriptionCount = 0;
  private heartbeatTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimeoutTimer: ReturnType<typeof setTimeout> | null = null;
  private lastMessageTime: number = Date.now();
  private metrics: ConnectionMetrics = {
    reconnectAttempts: 0,
    lastDisconnectTime: null,
    lastDisconnectReason: null,
    totalReconnects: 0,
    isHealthy: true,
  };
  private onMetricsChange?: (metrics: ConnectionMetrics) => void;

  constructor(
    private url: string,
    private onStatusChange?: (status: WebSocketStatus) => void,
    reconnectOptions?: ReconnectOptions
  ) {
    this.reconnectOptions = { ...DEFAULT_RECONNECT_OPTIONS, ...reconnectOptions };
  }

  getStatus() {
    return this.status;
  }

  connect() {
    if (this.socket) return;
    this.intentionalClose = false;
    this.clearReconnectTimer();
    this.setStatus('connecting');
    this.socket = new WebSocket(this.url);

    this.socket.onopen = () => {
      this.reconnectAttempts = 0;
      this.setStatus('open');
      this.flushQueue();
      this.resubscribe();
    };

    this.socket.onmessage = (event) => {
      // Backend batches multiple messages with newlines, so we need to split and handle each
      const parts = (event.data as string).split('\n');
      for (const part of parts) {
        const trimmed = part.trim();
        if (!trimmed) continue;

        try {
          const message = JSON.parse(trimmed) as BackendMessageMap[BackendMessageType];
          if (message.type === 'response' && (message as { id?: string }).id) {
            const msgId = (message as { id: string }).id;
            const pending = this.pendingRequests.get(msgId);
            if (pending) {
              clearTimeout(pending.timeout);
              this.pendingRequests.delete(msgId);
              pending.resolve(message.payload);
            }
            continue;
          }
          if (message.type === 'error' && (message as { id?: string }).id) {
            const msgId = (message as { id: string }).id;
            const pending = this.pendingRequests.get(msgId);
            if (pending) {
              clearTimeout(pending.timeout);
              this.pendingRequests.delete(msgId);
              const errorMessage =
                typeof message.payload === 'object' && message.payload && 'message' in message.payload
                  ? String((message.payload as { message?: string }).message)
                  : 'WebSocket request failed';
              pending.reject(new Error(errorMessage));
            }
            continue;
          }

          if (message.type !== 'notification') {
            continue;
          }
          const action = (message as { action?: string })?.action as BackendMessageType | undefined;
          if (!action) continue;
          console.log('[WS] Received notification:', action);
          const handlers = this.handlers.get(action);
          if (!handlers) {
            console.log('[WS] No handlers registered for:', action);
            continue;
          }
          console.log('[WS] Calling', handlers.size, 'handler(s) for:', action);
          handlers.forEach((handler) => handler(message));
        } catch {
          // Ignore parse errors for individual messages
        }
      }
    };

    this.socket.onerror = () => {
      this.setStatus('error');
    };

    this.socket.onclose = (event) => {
      this.socket = null;
      this.handleDisconnect(event);
    };
  }

  disconnect() {
    this.intentionalClose = true;
    this.clearReconnectTimer();
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.setStatus('closed');
    this.cleanupPendingRequests();
  }

  send(payload: unknown) {
    const data = JSON.stringify(payload);
    if (this.status !== 'open' || !this.socket) {
      this.pendingQueue.push(data);
      return;
    }
    this.socket.send(data);
  }

  request<T>(action: string, payload: unknown, timeoutMs = 5000): Promise<T> {
    const id = generateUUID();
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error(`WebSocket request timed out: ${action}`));
      }, timeoutMs);
      this.pendingRequests.set(id, { resolve: resolve as (payload: unknown) => void, reject, timeout });
      this.send({ id, type: 'request', action, payload });
    });
  }

  subscribe(taskId: string) {
    const currentCount = this.subscriptions.get(taskId) ?? 0;
    const nextCount = currentCount + 1;
    this.subscriptions.set(taskId, nextCount);
    if (this.status === 'open' && nextCount === 1) {
      this.send({
        id: generateUUID(),
        type: 'request',
        action: 'task.subscribe',
        payload: { task_id: taskId },
      });
    }
    return () => this.unsubscribe(taskId);
  }

  subscribeUser() {
    this.userSubscriptionCount += 1;
    if (this.status === 'open' && this.userSubscriptionCount === 1) {
      this.send({
        id: generateUUID(),
        type: 'request',
        action: 'user.subscribe',
        payload: {},
      });
    }
  }

  unsubscribe(taskId: string) {
    const currentCount = this.subscriptions.get(taskId);
    if (!currentCount) return;
    const nextCount = currentCount - 1;
    if (nextCount <= 0) {
      this.subscriptions.delete(taskId);
      if (this.status === 'open') {
        this.send({
          id: generateUUID(),
          type: 'request',
          action: 'task.unsubscribe',
          payload: { task_id: taskId },
        });
      }
      return;
    }
    this.subscriptions.set(taskId, nextCount);
  }

  unsubscribeUser() {
    this.userSubscriptionCount = Math.max(0, this.userSubscriptionCount - 1);
    if (this.status === 'open' && this.userSubscriptionCount === 0) {
      this.send({
        id: generateUUID(),
        type: 'request',
        action: 'user.unsubscribe',
        payload: {},
      });
    }
  }

  on<T extends BackendMessageType>(type: T, handler: MessageHandler<T>) {
    const handlers = this.handlers.get(type) ?? new Set();
    handlers.add(handler as MessageHandler<BackendMessageType>);
    this.handlers.set(type, handlers);
    return () => this.off(type, handler);
  }

  off<T extends BackendMessageType>(type: T, handler: MessageHandler<T>) {
    const handlers = this.handlers.get(type);
    if (!handlers) return;
    handlers.delete(handler as MessageHandler<BackendMessageType>);
    if (!handlers.size) {
      this.handlers.delete(type);
    }
  }

  private handleDisconnect(event: CloseEvent) {
    this.setStatus('closed');

    // Don't reconnect if this was an intentional close
    if (this.intentionalClose) {
      return;
    }

    // Don't reconnect if reconnect is disabled
    if (!this.reconnectOptions.enabled) {
      return;
    }

    // Don't reconnect if we've exceeded max attempts
    if (this.reconnectAttempts >= this.reconnectOptions.maxAttempts) {
      console.warn(`WebSocket max reconnect attempts (${this.reconnectOptions.maxAttempts}) reached`);
      this.setStatus('error');
      this.cleanupPendingRequests();
      return;
    }

    // Calculate delay with exponential backoff
    const delay = Math.min(
      this.reconnectOptions.initialDelay * Math.pow(this.reconnectOptions.backoffMultiplier, this.reconnectAttempts),
      this.reconnectOptions.maxDelay
    );

    this.reconnectAttempts++;
    this.setStatus('reconnecting');

    console.log(
      `WebSocket disconnected (code: ${event.code}, reason: ${event.reason || 'none'}). ` +
        `Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.reconnectOptions.maxAttempts})...`
    );

    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }

  private clearReconnectTimer() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private cleanupPendingRequests() {
    // Reject all pending requests
    this.pendingRequests.forEach(({ reject, timeout }) => {
      clearTimeout(timeout);
      reject(new Error('WebSocket connection closed'));
    });
    this.pendingRequests.clear();
  }

  private resubscribe() {
    // Re-subscribe to all tasks after reconnection
    this.subscriptions.forEach((_count, taskId) => {
      this.send({
        id: generateUUID(),
        type: 'request',
        action: 'task.subscribe',
        payload: { task_id: taskId },
      });
    });
    if (this.userSubscriptionCount > 0) {
      this.send({
        id: generateUUID(),
        type: 'request',
        action: 'user.subscribe',
        payload: {},
      });
    }
  }

  private flushQueue() {
    if (!this.socket || this.status !== 'open') return;
    this.pendingQueue.forEach((data) => this.socket?.send(data));
    this.pendingQueue = [];
  }

  private setStatus(status: WebSocketStatus) {
    this.status = status;
    this.onStatusChange?.(status);
  }
}
