export class NoopWebSocket {
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onopen: (() => void) | null = null;

  close() {
    this.onclose?.({ code: 1000 } as CloseEvent);
  }

  send() {}
}
