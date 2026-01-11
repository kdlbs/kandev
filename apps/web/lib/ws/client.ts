import type { BackendMessageMap, BackendMessageType } from '@/lib/types/backend';

type MessageHandler<T extends BackendMessageType> = (message: BackendMessageMap[T]) => void;

type WebSocketStatus = 'idle' | 'connecting' | 'open' | 'closed' | 'error';

export class WebSocketClient {
  private socket: WebSocket | null = null;
  private status: WebSocketStatus = 'idle';
  private handlers = new Map<BackendMessageType, Set<MessageHandler<BackendMessageType>>>();
  private pendingQueue: string[] = [];

  constructor(private url: string) {}

  getStatus() {
    return this.status;
  }

  connect() {
    if (this.socket) return;
    this.status = 'connecting';
    this.socket = new WebSocket(this.url);

    this.socket.onopen = () => {
      this.status = 'open';
      this.flushQueue();
    };

    this.socket.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data) as BackendMessageMap[BackendMessageType];
        const handlers = this.handlers.get(message.type);
        if (!handlers) return;
        handlers.forEach((handler) => handler(message));
      } catch {
        // Ignore malformed messages for now.
      }
    };

    this.socket.onerror = () => {
      this.status = 'error';
    };

    this.socket.onclose = () => {
      this.status = 'closed';
      this.socket = null;
    };
  }

  disconnect() {
    if (!this.socket) return;
    this.socket.close();
    this.socket = null;
    this.status = 'closed';
  }

  send(payload: unknown) {
    const data = JSON.stringify(payload);
    if (this.status !== 'open' || !this.socket) {
      this.pendingQueue.push(data);
      return;
    }
    this.socket.send(data);
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
}
