import type {
  ActionResult,
  ActionType,
  LoginResponse,
} from "./types";

interface ErrorPayload {
  code?: string;
  message?: string;
}

export async function login(): Promise<LoginResponse> {
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: 1, actor_id: 1, world_id: 1 }),
  });
  if (!res.ok) {
    const error = await readPayload<ErrorPayload>(res);
    throw new Error(error.message || error.code || `login failed: ${res.status}`);
  }
  return await readPayload<LoginResponse>(res);
}

export async function postAction(
  token: string,
  worldID: number,
  actionType: ActionType,
  patch: Partial<{ x: number; y: number }>,
): Promise<{ ok: boolean; data: ActionResult; status: number }> {
  const res = await fetch(`/api/v1/worlds/${worldID}/actions`, {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      action_type: actionType,
      client_action_id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
      ...patch,
    }),
  });
  return {
    ok: res.ok,
    data: await readPayload<ActionResult>(res),
    status: res.status,
  };
}

export async function connectStream(
  token: string,
  worldID: number,
  lastEventID: number,
  signal: AbortSignal,
  onEvent: (type: string, data: unknown, id: number) => void,
): Promise<void> {
  const headers: Record<string, string> = { Authorization: `Bearer ${token}` };
  if (lastEventID > 0) {
    headers["Last-Event-ID"] = String(lastEventID);
  }

  const res = await fetch(`/api/v1/worlds/${worldID}/stream`, {
    headers,
    signal,
  });
  if (!res.ok || !res.body) {
    const error = await readPayload<ErrorPayload>(res);
    throw new Error(error.message || error.code || `stream failed: ${res.status}`);
  }

  await readSSE(res.body, onEvent);
}

async function readPayload<T>(res: Response): Promise<T> {
  const text = await res.text();
  if (!text.trim()) {
    return {} as T;
  }
  try {
    return JSON.parse(text) as T;
  } catch {
    return { message: text } as T;
  }
}

async function readSSE(
  body: ReadableStream<Uint8Array>,
  onEvent: (type: string, data: unknown, id: number) => void,
): Promise<void> {
  const decoder = new TextDecoder();
  const reader = body.getReader();
  let buffer = "";

  while (true) {
    const { value, done } = await reader.read();
    if (done) {
      buffer += decoder.decode();
      if (buffer.trim()) {
        handleSSE(buffer, onEvent);
      }
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    let separator = findSSESeparator(buffer);
    while (separator) {
      const raw = buffer.slice(0, separator.index);
      buffer = buffer.slice(separator.index + separator.length);
      handleSSE(raw, onEvent);
      separator = findSSESeparator(buffer);
    }
  }
}

function findSSESeparator(buffer: string): { index: number; length: number } | undefined {
  const lf = buffer.indexOf("\n\n");
  const crlf = buffer.indexOf("\r\n\r\n");
  if (lf === -1 && crlf === -1) {
    return undefined;
  }
  if (crlf !== -1 && (lf === -1 || crlf < lf)) {
    return { index: crlf, length: 4 };
  }
  return { index: lf, length: 2 };
}

function handleSSE(
  raw: string,
  onEvent: (type: string, data: unknown, id: number) => void,
): void {
  const lines = raw.split(/\r?\n/);
  let id = 0;
  let event = "message";
  const data: string[] = [];
  for (const line of lines) {
    if (line.startsWith("id:")) id = Number(line.slice(3).trim());
    if (line.startsWith("event:")) event = line.slice(6).trim();
    if (line.startsWith("data:")) data.push(line.slice(5).trimStart());
  }
  if (data.length === 0) {
    return;
  }
  try {
    onEvent(event, JSON.parse(data.join("\n")) as unknown, id);
  } catch {
    onEvent("stream_error", { message: "invalid stream event" }, id);
  }
}

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
