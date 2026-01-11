import type { BackendMessageMap, BackendMessageType } from '@/lib/types/backend';

type MessageHandler<T extends BackendMessageType> = (message: BackendMessageMap[T]) => void;

type WebSocketStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private status: WebSocketStatus = 'idle';
  private handlers = new Map<BackendMessageType, Set<MessageHandler<BackendMessageType>>>();
  private pendingRequests = new Map<
    string,
    { resolve: (payload: unknown) => void; reject: (error: Error) => void; timeout: ReturnType<typeof setTimeout> }
  >();
  private pendingQueue: string[] = [];

  constructor(
    private url: string,
    private onStatusChange?: (status: WebSocketStatus) => void
  ) {}

  getStatus() {
    return this.status;
  }

  connect() {
    if (this.socket) return;
    this.setStatus('connecting');
    this.socket = new WebSocket(this.url);

    this.socket.onopen = () => {
      this.setStatus('open');
      this.flushQueue();
    };

    this.socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as BackendMessageMap[BackendMessageType];
        if (message.type === 'response' && message.id) {
          const pending = this.pendingRequests.get(message.id);
          if (pending) {
            clearTimeout(pending.timeout);
            this.pendingRequests.delete(message.id);
            pending.resolve(message.payload);
          }
          return;
        }
        if (message.type === 'error' && message.id) {
          const pending = this.pendingRequests.get(message.id);
          if (pending) {
            clearTimeout(pending.timeout);
            this.pendingRequests.delete(message.id);
            const errorMessage =
              typeof message.payload === 'object' && message.payload && 'message' in message.payload
                ? String((message.payload as { message?: string }).message)
                : 'WebSocket request failed';
            pending.reject(new Error(errorMessage));
          }
          return;
        }

        if (message.type !== 'notification') {
          return;
        }
        const action = message?.action;
        if (!action) return;
        const handlers = this.handlers.get(action);
        if (!handlers) return;
        handlers.forEach((handler) => handler(message));
      } catch {
        // Ignore malformed messages for now.
      }
    };

    this.socket.onerror = () => {
      this.setStatus('error');
    };

    this.socket.onclose = () => {
      this.setStatus('closed');
      this.socket = null;
    };
  }

  disconnect() {
    if (!this.socket) return;
    this.socket.close();
    this.socket = null;
    this.setStatus('closed');
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
    const id = crypto.randomUUID();
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error(`WebSocket request timed out: ${action}`));
      }, timeoutMs);
      this.pendingRequests.set(id, { resolve, reject, timeout });
      this.send({ id, type: 'request', action, payload });
    });
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
