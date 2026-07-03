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
import { readCellMeta } from "./cellMeta";
import { Panel } from "./components/Panel";
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
  LogEvent,
  PanelID,
  StatusKind,
  WorldCell,
} from "./types";

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
  const [hoveredCell, setHoveredCell] = createSignal<WorldCell | undefined>();
  const [selectedCell, setSelectedCell] = createSignal<WorldCell | undefined>();
  const [collapsedPanels, setCollapsedPanels] = createSignal<
    Partial<Record<PanelID, boolean>>
  >({ events: true });

  const actorPosition = createMemo(() => `${actor().x}, ${actor().y}`);
  const hoveredPosition = createMemo(() => {
    const cell = hoveredCell();
    return cell ? `${cell.x}, ${cell.y}` : "-";
  });
  const selectedCellMeta = createMemo(() => {
    const cell = selectedCell();
    return cell ? readCellMeta(chunks, cell) : undefined;
  });
  const selectedPosition = createMemo(() => {
    const cell = selectedCell();
    return cell ? `${cell.x}, ${cell.y}` : "-";
  });
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

  function trackHover(event: MouseEvent): void {
    setHoveredCell(renderer?.cellAtClientPoint(event.clientX, event.clientY));
  }

  function selectHoveredCell(event: MouseEvent): void {
    const cell = renderer?.cellAtClientPoint(event.clientX, event.clientY);
    setSelectedCell(cell);
  }

  function isPanelCollapsed(id: PanelID): boolean {
    return collapsedPanels()[id] || false;
  }

  function togglePanel(id: PanelID): void {
    setCollapsedPanels((current) => ({ ...current, [id]: !current[id] }));
  }

  return (
    <main class="app-root">
      <section class="map-layer" aria-label="Карта мира">
        <canvas
          ref={canvasRef}
          id="worldCanvas"
          width="768"
          height="768"
          onClick={selectHoveredCell}
          onMouseLeave={() => setHoveredCell(undefined)}
          onMouseMove={trackHover}
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
          <Panel
            title="Актёр"
            collapsed={isPanelCollapsed("actor")}
            summary={actorPosition()}
            contentClass="stats"
            onToggle={() => togglePanel("actor")}
          >
            <>
              <div>
                <span>Чанков</span>
                <strong>{chunkCount()}</strong>
              </div>
              <div>
                <span>Запас клетки</span>
                <strong>{cellStock()}</strong>
              </div>
              <div>
                <span>Курсор</span>
                <strong>{hoveredPosition()}</strong>
              </div>
              <div>
                <span>Инвентарь</span>
                <strong>{inventoryTotal()}</strong>
              </div>
            </>
          </Panel>

          <Panel
            title="Клетка"
            collapsed={isPanelCollapsed("cell")}
            summary={selectedPosition()}
            onToggle={() => togglePanel("cell")}
          >
            {selectedCell() ? (
              selectedCellMeta() ? (
                <dl class="cell-meta">
                  <div>
                    <dt>Биом</dt>
                    <dd>{selectedCellMeta()?.biome}</dd>
                  </div>
                  <div>
                    <dt>Почва</dt>
                    <dd>{selectedCellMeta()?.soil}</dd>
                  </div>
                  <div>
                    <dt>Вода</dt>
                    <dd>
                      {selectedCellMeta()?.water}
                      {selectedCellMeta()?.waterTidal ? " tidal" : ""}
                    </dd>
                  </div>
                  <div>
                    <dt>Покров</dt>
                    <dd>{selectedCellMeta()?.cover}</dd>
                  </div>
                  <div>
                    <dt>Высота</dt>
                    <dd>
                      {selectedCellMeta()?.height}/
                      {selectedCellMeta()?.elevation}
                    </dd>
                  </div>
                  <div>
                    <dt>Запас</dt>
                    <dd>{selectedCellMeta()?.stock}</dd>
                  </div>
                  <div>
                    <dt>Уровни</dt>
                    <dd>
                      w{selectedCellMeta()?.waterLevel} c
                      {selectedCellMeta()?.coverLevel}
                    </dd>
                  </div>
                  <div>
                    <dt>Флаги</dt>
                    <dd>
                      b{selectedCellMeta()?.baseFlags} c
                      {selectedCellMeta()?.coverFlags}
                    </dd>
                  </div>
                  <div>
                    <dt>Чанк</dt>
                    <dd>
                      {selectedCellMeta()?.cx}, {selectedCellMeta()?.cy} #
                      {selectedCellMeta()?.index}
                    </dd>
                  </div>
                  <div>
                    <dt>Тик</dt>
                    <dd>{selectedCellMeta()?.updatedTick}</dd>
                  </div>
                </dl>
              ) : (
                <p class="panel-empty">чанк не загружен</p>
              )
            ) : (
              <p class="panel-empty">кликни по карте</p>
            )}
          </Panel>

          <Panel
            title="Карман"
            collapsed={isPanelCollapsed("inventory")}
            summary={inventory().length}
            onToggle={() => togglePanel("inventory")}
          >
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
          </Panel>

          <Panel
            title="Управление"
            collapsed={isPanelCollapsed("controls")}
            contentClass="controls"
            onToggle={() => togglePanel("controls")}
          >
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
          </Panel>

          <Panel
            title="События"
            collapsed={isPanelCollapsed("events")}
            summary={events().length}
            actions={
              <button
                type="button"
                disabled={busy()}
                class="text-button"
                onClick={startStream}
              >
                обновить
              </button>
            }
            onToggle={() => togglePanel("events")}
          >
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
          </Panel>
        </aside>
      </section>
    </main>
  );
}
