type MessageHandler = (payload: unknown) => void;

export interface WSMessage {
  type: string;
  payload: unknown;
}

export class CanvasWebSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<MessageHandler>>();
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 10;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private intentionallyClosed = false;

  constructor(private projectId: string) {}

  connect() {
    this.intentionallyClosed = false;
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const url = `${protocol}//${host}/api/projects/${this.projectId}/canvas/ws`;

    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      // Send auth as first message.
      const token = localStorage.getItem("access_token");
      if (token) {
        this.sendRaw({ type: "auth", payload: { token } });
      }
    };

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data);
        const handlers = this.handlers.get(msg.type);
        if (handlers) {
          handlers.forEach((handler) => handler(msg.payload));
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    this.ws.onclose = (event) => {
      if (!this.intentionallyClosed && event.code !== 1000) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = () => {
      // onclose will fire after this.
    };
  }

  on<T = unknown>(type: string, handler: (payload: T) => void): () => void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set());
    }
    this.handlers.get(type)!.add(handler as MessageHandler);
    return () => {
      this.handlers.get(type)?.delete(handler as MessageHandler);
    };
  }

  send(type: string, payload: unknown) {
    this.sendRaw({ type, payload });
  }

  disconnect() {
    this.intentionallyClosed = true;
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    this.ws?.close(1000);
    this.ws = null;
  }

  private sendRaw(msg: WSMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  private scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      return;
    }

    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000);
    this.reconnectAttempts++;

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
    }, delay);
  }
}
