# WebSocket Client with Auto-Reconnect

This directory contains the WebSocket client implementation with automatic reconnection capabilities.

## Features

### 1. Automatic Reconnection
- **Exponential Backoff**: Reconnection delays increase exponentially to avoid overwhelming the server
- **Configurable Retry Limits**: Set maximum number of reconnection attempts
- **Graceful Degradation**: Properly handles connection failures and notifies the UI

### 2. Connection State Management
The client tracks the following connection states:
- `idle`: Initial state before any connection attempt
- `connecting`: Actively establishing a connection
- `open`: Successfully connected and ready to send/receive messages
- `reconnecting`: Attempting to reconnect after a disconnect
- `closed`: Connection closed (intentionally or after max retries)
- `error`: Connection failed and won't retry

### 3. Message Queue
- Messages sent while disconnected are queued
- Queued messages are automatically sent when connection is restored
- Prevents message loss during brief disconnections

### 4. Subscription Management
- Tracks task subscriptions across reconnections
- Automatically re-subscribes to all tasks when reconnected
- Provides `subscribe()` and `unsubscribe()` methods for easy subscription management

### 5. Pending Request Handling
- Properly cleans up pending requests on disconnect
- Rejects pending promises with appropriate error messages
- Prevents memory leaks from abandoned requests

## Configuration

### Reconnect Options

```typescript
interface ReconnectOptions {
  enabled?: boolean;           // Enable/disable auto-reconnect (default: true)
  maxAttempts?: number;        // Maximum reconnection attempts (default: 10)
  initialDelay?: number;       // Initial delay in ms (default: 1000)
  maxDelay?: number;           // Maximum delay in ms (default: 30000)
  backoffMultiplier?: number;  // Exponential backoff multiplier (default: 1.5)
}
```

### Default Configuration

```typescript
const DEFAULT_RECONNECT_OPTIONS = {
  enabled: true,
  maxAttempts: 10,
  initialDelay: 1000,      // 1 second
  maxDelay: 30000,         // 30 seconds
  backoffMultiplier: 1.5,
};
```

## Usage

### Basic Usage

```typescript
import { WebSocketClient } from '@/lib/ws/client';

const client = new WebSocketClient(
  'ws://localhost:8080/ws',
  (status) => {
    console.log('Connection status:', status);
  }
);

client.connect();
```

### Custom Reconnect Configuration

```typescript
const client = new WebSocketClient(
  'ws://localhost:8080/ws',
  (status) => {
    console.log('Connection status:', status);
  },
  {
    enabled: true,
    maxAttempts: 5,
    initialDelay: 2000,
    maxDelay: 60000,
    backoffMultiplier: 2,
  }
);
```

### Subscribing to Tasks

```typescript
// Subscribe to task updates
client.subscribe('task-id-123');

// Unsubscribe when done
client.unsubscribe('task-id-123');
```

### Sending Messages

```typescript
// Send a message (queued if not connected)
client.send({ type: 'request', action: 'some.action', payload: {} });

// Send a request and wait for response
const response = await client.request('task.get', { id: 'task-123' });
```

### Listening to Events

```typescript
const unsubscribe = client.on('task.updated', (message) => {
  console.log('Task updated:', message.payload);
});

// Clean up when done
unsubscribe();
```

## Reconnection Behavior

### Exponential Backoff Example

With default settings:
- Attempt 1: 1 second delay
- Attempt 2: 1.5 seconds delay
- Attempt 3: 2.25 seconds delay
- Attempt 4: 3.375 seconds delay
- ...
- Attempt 10: 30 seconds delay (capped at maxDelay)

### What Happens on Disconnect

1. Connection closes (network issue, server restart, etc.)
2. Client sets status to `closed`
3. If not an intentional disconnect and reconnect is enabled:
   - Calculate delay using exponential backoff
   - Set status to `reconnecting`
   - Schedule reconnection attempt
4. On successful reconnection:
   - Reset reconnect attempt counter
   - Set status to `open`
   - Flush queued messages
   - Re-subscribe to all tasks
5. If max attempts reached:
   - Set status to `error`
   - Clean up pending requests
   - Stop reconnection attempts

## Integration with React

The `useWebSocket` hook automatically sets up the client with reconnection:

```typescript
import { useWebSocket } from '@/lib/ws/use-websocket';

export function MyComponent() {
  const store = useAppStoreApi();
  useWebSocket(store, 'ws://localhost:8080/ws');
  
  // Connection status is automatically synced to store
  const status = useAppStore((s) => s.connection.status);
  
  return <div>Status: {status}</div>;
}
```

## Testing

To test reconnection behavior:

1. Start the application
2. Open browser DevTools → Network tab
3. Observe WebSocket connection
4. Stop the backend server
5. Watch the client attempt to reconnect with increasing delays
6. Restart the backend server
7. Verify the client reconnects and re-subscribes to tasks

## Best Practices

1. **Always use the provided methods**: Use `subscribe()`/`unsubscribe()` instead of sending raw messages
2. **Handle connection states in UI**: Show appropriate feedback for `connecting`, `reconnecting`, and `error` states
3. **Clean up subscriptions**: Always unsubscribe when components unmount
4. **Configure appropriately**: Adjust reconnect settings based on your use case
5. **Monitor console logs**: Reconnection attempts are logged for debugging

