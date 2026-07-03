import { createMemo, createSignal, For, onCleanup, onMount } from "solid-js";
import {
  connectStream,
  errorMessage,
  isAbortError,
  login,
  postAction,
} from "./api";
import {
  actorChunkAndIndex,
  chunkKey,
  normalizeActor,
  normalizeChunk,
  normalizeInventory,
} from "./chunks";
import { MAX_ZOOM, MIN_ZOOM } from "./config";
import { MapRenderer } from "./mapRenderer";
import type {
  ActionType,
  Actor,
  ChunkSnapshot,
  ChunkSnapshotWire,
  EntityPatchPayload,
  HelloPayload,
  InventoryItem,
  StatusKind,
} from "./types";

interface LogEvent {
  id: number;
  title: string;
  detail?: string | number;
}

export function App() {
  let canvasRef!: HTMLCanvasElement;
  let renderer: MapRenderer | undefined;
  let streamAbort: AbortController | undefined;
  let nextLogID = 1;
  let token = "";
  let lastEventID = 0;

  const chunks = new Map<string, ChunkSnapshot>();

  const [status, setStatus] = createSignal<{ kind: StatusKind; text: string }>({
    kind: "idle",
    text: "загрузка",
  });
  const [actor, setActor] = createSignal<Actor>({
    id: 1,
    world_id: 1,
    x: 900,
    y: 1900,
  });
  const [worldID, setWorldID] = createSignal(1);
  const [chunkCount, setChunkCount] = createSignal(0);
  const [cellStock, setCellStock] = createSignal("-");
  const [busy, setBusy] = createSignal(false);
  const [hudHidden, setHudHidden] = createSignal(false);
  const [events, setEvents] = createSignal<LogEvent[]>([]);
  const [inventory, setInventory] = createSignal<InventoryItem[]>([]);

  const actorPosition = createMemo(() => `${actor().x}, ${actor().y}`);
  const subtitle = createMemo(() => `Пользователь #${actor().id}`);
  const inventoryTotal = createMemo(() =>
    inventory().reduce((sum, item) => sum + item.amount, 0),
  );

  onMount(() => {
    renderer = new MapRenderer(canvasRef);

    const resize = () => renderer?.resize(actor(), chunks);
    const keydown = (event: KeyboardEvent) => {
      if (event.repeat || busy()) {
        return;
      }
      if (event.key === "ArrowUp" || event.key.toLowerCase() === "w")
        move(0, -1);
      if (event.key === "ArrowDown" || event.key.toLowerCase() === "s")
        move(0, 1);
      if (event.key === "ArrowLeft" || event.key.toLowerCase() === "a")
        move(-1, 0);
      if (event.key === "ArrowRight" || event.key.toLowerCase() === "d")
        move(1, 0);
      if (event.key === " " || event.key.toLowerCase() === "e")
        void action("harvest", {});
    };

    window.addEventListener("resize", resize);
    window.addEventListener("keydown", keydown);
    void boot();

    onCleanup(() => {
      streamAbort?.abort();
      window.removeEventListener("resize", resize);
      window.removeEventListener("keydown", keydown);
    });
  });

  function addEvent(title: string, detail?: string | number): void {
    setEvents((current) =>
      [{ id: nextLogID++, title, detail }, ...current].slice(0, 40),
    );
  }

  function refreshStats(): void {
    const pos = actorChunkAndIndex(actor());
    const chunk = chunks.get(chunkKey(pos.cx, pos.cy));
    setChunkCount(chunks.size);
    setCellStock(chunk ? String(chunk.stock[pos.index] || 0) : "-");
  }

  function redraw(): void {
    refreshStats();
    renderer?.draw(actor(), chunks);
  }

  async function boot(): Promise<void> {
    try {
      setStatus({ kind: "idle", text: "вход" });
      const data = await login();
      token = data.token;
      setWorldID(data.worlds[0]?.id || 1);
      if (data.actors[0]) {
        setActor(normalizeActor(data.actors[0], worldID()));
      }
      setInventory(normalizeInventory(data.inventory));
      addEvent("Вход выполнен", `world=${worldID()}`);
      refreshStats();
      renderer?.resize(actor(), chunks);
      startStream();
    } catch (err) {
      setStatus({ kind: "error", text: "ошибка входа" });
      addEvent("Ошибка", errorMessage(err));
    }
  }

  function startStream(): void {
    streamAbort?.abort();
    streamAbort = new AbortController();
    setStatus({ kind: "idle", text: "подключение" });
    connectStream(token, worldID(), lastEventID, streamAbort.signal, applyEvent)
      .then(() => setStatus({ kind: "idle", text: "отключено" }))
      .catch((err: unknown) => {
        if (!isAbortError(err)) {
          setStatus({ kind: "error", text: "сервер недоступен" });
          addEvent("Стрим", errorMessage(err));
        }
      });
    setStatus({ kind: "live", text: "онлайн" });
  }

  function applyEvent(type: string, data: unknown, id: number): void {
    if (id > 0) {
      lastEventID = id;
    }
    if (type === "hello") {
      const hello = data as HelloPayload;
      if (hello.render_config) {
        renderer?.setRenderConfig(hello.render_config);
      }
      if (hello.actor) {
        setActor(normalizeActor(hello.actor, worldID()));
        redraw();
      }
      setInventory(normalizeInventory(hello.inventory));
      addEvent("Стрим открыт", `actor=${hello.actor_id}`);
      return;
    }
    if (type === "chunk_snapshot") {
      const chunk = normalizeChunk(data as ChunkSnapshotWire);
      chunks.set(chunkKey(chunk.cx, chunk.cy), chunk);
      redraw();
      return;
    }
    if (type === "entity_patch") {
      const patch = data as EntityPatchPayload | undefined;
      if (patch?.actor) {
        const nextActor = normalizeActor(patch.actor, worldID());
        setActor(nextActor);
        setInventory(normalizeInventory(patch.inventory));
        addEvent(
          "Перемещение",
          `x=${nextActor.x}, y=${nextActor.y}, event=${id || patch.event_id || 0}`,
        );
      }
      redraw();
    }
  }

  async function action(
    actionType: ActionType,
    patch: Partial<{ x: number; y: number }>,
  ): Promise<void> {
    if (busy()) {
      return;
    }
    setBusy(true);
    try {
      const result = await postAction(token, worldID(), actionType, patch);
      if (!result.ok) {
        addEvent(
          "Ошибка",
          result.data.message || result.data.code || result.status,
        );
        return;
      }
      if (result.data.actor) {
        setActor(normalizeActor(result.data.actor, worldID()));
      }
      if (result.data.inventory) {
        setInventory(normalizeInventory(result.data.inventory));
      }
      if (actionType === "harvest") {
        addEvent("Сбор", `event=${result.data.event_id || 0}`);
      }
      redraw();
    } finally {
      setBusy(false);
    }
  }

  function move(dx: number, dy: number): void {
    const current = actor();
    void action("move", { x: current.x + dx, y: current.y + dy });
  }

  function zoomCanvas(event: WheelEvent): void {
    event.preventDefault();
    renderer?.zoom(event.deltaY, MIN_ZOOM, MAX_ZOOM, actor(), chunks);
  }

  return (
    <main class="app-root">
      <section class="map-layer" aria-label="Карта мира">
        <canvas
          ref={canvasRef}
          id="worldCanvas"
          width="768"
          height="768"
          onWheel={zoomCanvas}
        />
        <div classList={{ "map-empty": true, hidden: chunkCount() > 0 }}>
          ожидание чанков
        </div>
      </section>

      <button
        class="hud-toggle"
        type="button"
        aria-label={hudHidden() ? "Показать HUD" : "Скрыть HUD"}
        title={hudHidden() ? "Показать HUD" : "Скрыть HUD"}
        onClick={() => setHudHidden((hidden) => !hidden)}
      >
        {hudHidden() ? "+" : "×"}
      </button>

      <section
        classList={{ "hud-layer": true, hidden: hudHidden() }}
        aria-label="Игровой HUD"
      >
        <div>
          <h1 class="brand-title">
            <img
              class="brand-mark"
              src="/assets/logobw.png"
              alt="Chatti's Islands"
            />
          </h1>
        </div>

        <div
          classList={{
            status: true,
            live: status().kind === "live",
            error: status().kind === "error",
          }}
        >
          <span class="status-dot" />
          <span>{status().text}</span>
        </div>

        <aside class="sidebar" aria-label="Панель управления">
          <section class="panel">
            <div class="panel-title">
              <span>Актёр</span>
              <strong>{actorPosition()}</strong>
            </div>
            <div class="stats">
              <div>
                <span>Чанков</span>
                <strong>{chunkCount()}</strong>
              </div>
              <div>
                <span>Запас клетки</span>
                <strong>{cellStock()}</strong>
              </div>
              <div>
                <span>Инвентарь</span>
                <strong>{inventoryTotal()}</strong>
              </div>
            </div>
          </section>

          <section class="panel">
            <div class="panel-title">
              <span>Карман</span>
              <strong>{inventory().length}</strong>
            </div>
            <ol class="inventory-list">
              <For each={inventory()} fallback={<li>пусто</li>}>
                {(item) => (
                  <li>
                    <b>{item.name}</b>
                    <span>{item.amount}</span>
                  </li>
                )}
              </For>
            </ol>
          </section>

          <section class="panel controls">
            <button
              class="icon-button"
              id="moveUp"
              type="button"
              disabled={busy()}
              aria-label="Вверх"
              title="Вверх"
              onClick={() => move(0, -1)}
            >
              ↑
            </button>
            <button
              class="icon-button"
              id="moveLeft"
              type="button"
              disabled={busy()}
              aria-label="Влево"
              title="Влево"
              onClick={() => move(-1, 0)}
            >
              ←
            </button>
            <button
              class="icon-button primary"
              id="harvest"
              type="button"
              disabled={busy()}
              aria-label="Собрать ресурс"
              title="Собрать ресурс"
              onClick={() => void action("harvest", {})}
            >
              ⛏
            </button>
            <button
              class="icon-button"
              id="moveRight"
              type="button"
              disabled={busy()}
              aria-label="Вправо"
              title="Вправо"
              onClick={() => move(1, 0)}
            >
              →
            </button>
            <button
              class="icon-button"
              id="moveDown"
              type="button"
              disabled={busy()}
              aria-label="Вниз"
              title="Вниз"
              onClick={() => move(0, 1)}
            >
              ↓
            </button>
          </section>

          <section class="panel">
            <div class="panel-title">
              <span>События</span>
              <button
                type="button"
                disabled={busy()}
                class="text-button"
                onClick={startStream}
              >
                обновить
              </button>
            </div>
            <ol class="event-log">
              <For each={events()}>
                {(event) => (
                  <li>
                    <b>{event.title}</b>
                    {event.detail ? ` ${event.detail}` : ""}
                  </li>
                )}
              </For>
            </ol>
          </section>
        </aside>
      </section>
    </main>
  );
}
