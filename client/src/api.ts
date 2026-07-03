import type { ActionResult, ActionType, LoginResponse, RealtimeEvent } from "./types";

export async function login(): Promise<LoginResponse> {
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ user_id: 1, actor_id: 1, world_id: 1 }),
  });
  if (!res.ok) {
    throw new Error(`login failed: ${res.status}`);
  }
  return await res.json() as LoginResponse;
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
    data: await res.json() as ActionResult,
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

  const res = await fetch(`/api/v1/worlds/${worldID}/stream`, { headers, signal });
  if (!res.ok || !res.body) {
    throw new Error(`stream failed: ${res.status}`);
  }

  await readSSE(res.body, onEvent);
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
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    let splitAt = buffer.indexOf("\n\n");
    while (splitAt !== -1) {
      const raw = buffer.slice(0, splitAt);
      buffer = buffer.slice(splitAt + 2);
      handleSSE(raw, onEvent);
      splitAt = buffer.indexOf("\n\n");
    }
  }
}

function handleSSE(raw: string, onEvent: (type: string, data: unknown, id: number) => void): void {
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
  const payload = JSON.parse(data.join("\n")) as RealtimeEvent;
  onEvent(payload.type || event, payload.data, payload.id || id);
}

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
