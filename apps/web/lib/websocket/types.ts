// Mirror backend protocol types from apps/backend/pkg/websocket/message.go
export type MessageType = 'request' | 'response' | 'notification' | 'error';
export type ConnectionState = 'disconnected' | 'connecting' | 'connected' | 'error';

export interface WebSocketMessage {
  id?: string;
  type: MessageType;
  action: string;
  payload: any;
  timestamp: string;
}

export interface ErrorPayload {
  code: string;
  message: string;
  details?: Record<string, any>;
}

export interface WebSocketContextValue {
  state: ConnectionState;
  error: string | null;
  connect: () => void;
  disconnect: () => void;
  reconnect: () => void;
  sendMessage: (message: WebSocketMessage) => void;
}
