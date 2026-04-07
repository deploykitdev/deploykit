type MessageHandler = (payload: unknown) => void;

export interface WSMessage {
  type: string;
  payload: unknown;
}

export type ConnectionStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "disconnected";

type StatusHandler = (status: ConnectionStatus) => void;

export class CanvasWebSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<MessageHandler>>();
  private statusHandlers = new Set<StatusHandler>();
  private status: ConnectionStatus = "disconnected";
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 15;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private intentionallyClosed = false;

  constructor(private projectId: string) {}

  connect() {
    // Tear down any existing socket so reconnect/manual-retry can't leave a
    // stale connection alive (which would make the user appear twice on the
    // server).
    if (this.ws) {
      // Detach handlers so the old socket's close event doesn't trigger
      // another reconnect cycle or status flip.
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;
      try {
        this.ws.close(1000);
      } catch {
        // ignore
      }
      this.ws = null;
    }

    this.intentionallyClosed = false;
    this.setStatus("connecting");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const host = window.location.host;
    const url = `${protocol}//${host}/api/projects/${this.projectId}/canvas/ws`;

    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.reconnectAttempts = 0;
      this.setStatus("connected");
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
      if (this.intentionallyClosed || event.code === 1000) {
        this.setStatus("disconnected");
        return;
      }
      if (this.reconnectAttempts >= this.maxReconnectAttempts) {
        this.setStatus("disconnected");
        return;
      }
      this.setStatus("reconnecting");
      this.scheduleReconnect();
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

  // onStatusChange subscribes to connection status transitions. The handler is
  // called immediately with the current status and on every subsequent change.
  // Returns an unsubscribe function.
  onStatusChange(handler: StatusHandler): () => void {
    this.statusHandlers.add(handler);
    handler(this.status);
    return () => {
      this.statusHandlers.delete(handler);
    };
  }

  // reconnect resets the retry counter and attempts to connect immediately.
  // Use this to wire a manual "retry" action from the UI after the socket
  // has given up.
  reconnect() {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    this.reconnectAttempts = 0;
    this.connect();
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
    this.setStatus("disconnected");
  }

  private sendRaw(msg: WSMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    }
  }

  private setStatus(status: ConnectionStatus) {
    if (this.status === status) return;
    this.status = status;
    this.statusHandlers.forEach((handler) => handler(status));
  }

  private scheduleReconnect() {
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000);
    this.reconnectAttempts++;

    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
    }, delay);
  }
}
