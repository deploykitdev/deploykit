import { useEffect, useRef, useState } from "react";
import { tryRefresh } from "./api";

const MAX_LINES = 5000;

export type LogStreamStatus =
  | "idle"
  | "connecting"
  | "streaming"
  | "error"
  | "ended";

export interface LogEntry {
  id: number;
  containerId: string;
  stream: "stdout" | "stderr";
  line: string;
}

export interface LogEvent {
  id: number;
  containerId: string;
  kind: "container_exited";
}

export type LogItem = LogEntry | LogEvent;

export interface UseLogStreamResult {
  items: LogItem[];
  status: LogStreamStatus;
  error: string | null;
  clear: () => void;
  reconnect: () => void;
}

export function useLogStream(
  projectId: string,
  serviceId: string,
  enabled: boolean,
): UseLogStreamResult {
  const [items, setItems] = useState<LogItem[]>([]);
  const [status, setStatus] = useState<LogStreamStatus>("idle");
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  const idRef = useRef(0);

  useEffect(() => {
    setItems([]);
    setError(null);
    setStatus("idle");
  }, [projectId, serviceId]);

  useEffect(() => {
    if (!enabled) {
      setStatus("idle");
      return;
    }

    const controller = new AbortController();
    let cancelled = false;

    setStatus("connecting");
    setError(null);

    const pushEntry = (entry: Omit<LogEntry, "id">) => {
      setItems((prev) => {
        const next = prev.concat({ ...entry, id: idRef.current++ });
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
      });
    };

    const pushEvent = (ev: Omit<LogEvent, "id">) => {
      setItems((prev) => {
        const next = prev.concat({ ...ev, id: idRef.current++ });
        return next.length > MAX_LINES ? next.slice(next.length - MAX_LINES) : next;
      });
    };

    const handlePayload = (raw: string) => {
      let data: Record<string, unknown>;
      try {
        data = JSON.parse(raw);
      } catch {
        return;
      }
      if (data.event === "container_exited" && typeof data.container_id === "string") {
        pushEvent({ containerId: data.container_id, kind: "container_exited" });
        return;
      }
      if (
        typeof data.container_id === "string" &&
        typeof data.line === "string" &&
        typeof data.stream === "string"
      ) {
        pushEntry({
          containerId: data.container_id,
          stream: data.stream === "stderr" ? "stderr" : "stdout",
          line: data.line,
        });
      }
    };

    const parseEvent = (block: string) => {
      const dataLines: string[] = [];
      let eventName = "";
      for (const rawLine of block.split("\n")) {
        if (!rawLine || rawLine.startsWith(":")) continue;
        const idx = rawLine.indexOf(":");
        if (idx === -1) continue;
        const field = rawLine.slice(0, idx);
        const value = rawLine.slice(idx + 1).replace(/^ /, "");
        if (field === "data") dataLines.push(value);
        else if (field === "event") eventName = value;
      }
      if (eventName === "end") {
        if (!cancelled) setStatus("ended");
        return;
      }
      if (dataLines.length > 0) handlePayload(dataLines.join("\n"));
    };

    const run = async () => {
      const open = async (token: string | null) => {
        const headers: Record<string, string> = { Accept: "text/event-stream" };
        if (token) headers.Authorization = `Bearer ${token}`;
        return fetch(
          `/api/projects/${projectId}/services/${serviceId}/logs?tail=500`,
          { headers, signal: controller.signal },
        );
      };

      try {
        let token = localStorage.getItem("access_token");
        let res = await open(token);

        if (res.status === 401) {
          const refreshed = await tryRefresh();
          if (refreshed) {
            token = localStorage.getItem("access_token");
            res = await open(token);
          }
        }

        if (!res.ok || !res.body) {
          const text = await res.text().catch(() => "");
          if (!cancelled) {
            setError(text || `Failed to open log stream (${res.status})`);
            setStatus("error");
          }
          return;
        }

        if (!cancelled) setStatus("streaming");

        const reader = res.body.getReader();
        const decoder = new TextDecoder("utf-8");
        let buffer = "";

        while (!cancelled) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          let idx: number;
          while ((idx = buffer.indexOf("\n\n")) !== -1) {
            const block = buffer.slice(0, idx);
            buffer = buffer.slice(idx + 2);
            if (block.length > 0) parseEvent(block);
          }
        }

        if (!cancelled) setStatus((s) => (s === "ended" ? "ended" : "ended"));
      } catch (err) {
        if (cancelled || (err instanceof DOMException && err.name === "AbortError")) {
          return;
        }
        setError(err instanceof Error ? err.message : String(err));
        setStatus("error");
      }
    };

    run();

    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [projectId, serviceId, enabled, nonce]);

  return {
    items,
    status,
    error,
    clear: () => setItems([]),
    reconnect: () => setNonce((n) => n + 1),
  };
}
