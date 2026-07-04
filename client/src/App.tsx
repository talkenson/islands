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
  InventoryPatchPayload,
  InventoryItem,
  LogEvent,
  PanelID,
  StatusKind,
  WorldTime,
  WorldCell,
} from "./types";

const DEFAULT_WORLD_TIME: WorldTime = {
  world_time: 0,
  day: 1,
  phase: "late_night",
  phase_progress: 0,
  day_progress: 0,
  day_length_seconds: 480,
  world_seconds_per_real_second: 1,
};

const DAY_PHASES = [
  "late_night",
  "dawn",
  "morning",
  "day",
  "afternoon",
  "dusk",
  "evening",
  "night",
] as const;

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
    inventory_id: 1,
  });
  const [worldID, setWorldID] = createSignal(1);
  const [chunkCount, setChunkCount] = createSignal(0);
  const [cellStock, setCellStock] = createSignal("-");
  const [busy, setBusy] = createSignal(false);
  const [hudHidden, setHudHidden] = createSignal(false);
  const [events, setEvents] = createSignal<LogEvent[]>([]);
  const [inventory, setInventory] = createSignal<InventoryItem[]>([]);
  const [worldTime, setWorldTime] = createSignal<WorldTime>(DEFAULT_WORLD_TIME);
  const [hoveredCell, setHoveredCell] = createSignal<WorldCell | undefined>();
  const [selectedCell, setSelectedCell] = createSignal<WorldCell | undefined>();
  const [worldRevision, setWorldRevision] = createSignal(0);
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
    worldRevision();
    return cell ? readCellMeta(chunks, cell) : undefined;
  });
  const selectedPosition = createMemo(() => {
    const cell = selectedCell();
    return cell ? `${cell.x}, ${cell.y}` : "-";
  });
  const subtitle = createMemo(() => `Пользователь #${actor().id}`);
  const daySummary = createMemo(
    () => `День ${worldTime().day}, ${phaseLabel(worldTime().phase)}`,
  );
  const inventoryTotal = createMemo(() =>
    inventory().reduce((sum, item) => sum + item.amount, 0),
  );

  onMount(() => {
    renderer = new MapRenderer(canvasRef);
    let lastClockUpdate = Date.now();

    const resize = () => renderer?.resize(actor(), chunks, worldTime());
    const clock = window.setInterval(() => {
      const now = Date.now();
      const realSeconds = (now - lastClockUpdate) / 1000;
      lastClockUpdate = now;
      setWorldTime((current) => advanceWorldTime(current, realSeconds));
      redraw();
    }, 1000);
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
      if (event.key.toLowerCase() === "p") void action("plant_tree", {});
    };

    window.addEventListener("resize", resize);
    window.addEventListener("keydown", keydown);
    void boot();

    onCleanup(() => {
      streamAbort?.abort();
      renderer?.destroy();
      window.clearInterval(clock);
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
    renderer?.draw(actor(), chunks, worldTime());
  }

  async function boot(): Promise<void> {
    try {
      setStatus({ kind: "idle", text: "вход" });
      const data = await login();
      token = data.token;
      setWorldID(data.worlds[0]?.id || 1);
      if (data.actors[0]) {
        setActor(mergeActor(actor(), data.actors[0]));
      }
      setInventory(normalizeInventory(data.inventory));
      addEvent("Вход выполнен", `world=${worldID()}`);
      refreshStats();
      renderer?.resize(actor(), chunks, worldTime());
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
      if (hello.world_time) {
        setWorldTime(normalizeWorldTime(hello.world_time));
      }
      if (hello.actor) {
        setActor(mergeActor(actor(), hello.actor));
        redraw();
      }
      setInventory(normalizeInventory(hello.inventory));
      addEvent("Стрим открыт", `actor=${hello.actor_id}`);
      return;
    }
    if (type === "chunk_snapshot") {
      const chunk = normalizeChunk(data as ChunkSnapshotWire);
      chunks.set(chunkKey(chunk.cx, chunk.cy), chunk);
      setWorldRevision((revision) => revision + 1);
      redraw();
      return;
    }
    if (type === "world_time") {
      setWorldTime(normalizeWorldTime(data as WorldTime));
      redraw();
      return;
    }
    if (type === "entity_patch") {
      const patch = data as EntityPatchPayload | undefined;
      if (patch?.actor) {
        const previousActor = actor();
        const nextActor = mergeActor(previousActor, patch.actor);
        setActor(nextActor);
        if (
          previousActor.x !== nextActor.x ||
          previousActor.y !== nextActor.y
        ) {
          addEvent(
            "Перемещение",
            `x=${nextActor.x}, y=${nextActor.y}, event=${id}`,
          );
        }
      }
      redraw();
      return;
    }
    if (type === "inventory_patch") {
      const patch = data as InventoryPatchPayload | undefined;
      if (!patch || patch.actor_id !== actor().id) {
        return;
      }
      setInventory(normalizeInventory(patch.inventory));
      return;
    }
    if (type === "stream_error") {
      const payload = data as { message?: string } | undefined;
      addEvent("Стрим", payload?.message || "ошибка события");
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
    } catch (err) {
      setStatus({ kind: "error", text: "ошибка действия" });
      addEvent("Ошибка", errorMessage(err));
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
    renderer?.zoom(
      event.deltaY,
      MIN_ZOOM,
      MAX_ZOOM,
      actor(),
      chunks,
      worldTime(),
    );
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
              <div>
                <span>Время</span>
                <strong>{daySummary()}</strong>
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
                    <dt>Температура</dt>
                    <dd>{selectedCellMeta()?.temperature} °C</dd>
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
              id="plantTree"
              type="button"
              disabled={busy()}
              aria-label="Посадить дерево"
              title="Посадить дерево"
              onClick={() => void action("plant_tree", {})}
            >
              ♧
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

        <section class="pocket-bar" aria-label="Карман">
          <ol class="pocket-slots">
            <For each={inventory()} fallback={<li class="pocket-slot empty">пусто</li>}>
              {(item) => (
                <li class="pocket-slot">
                  <b>{item.name}</b>
                  {item.amount > 1 ? <span>{item.amount}</span> : null}
                </li>
              )}
            </For>
          </ol>
        </section>
      </section>
    </main>
  );
}

function phaseLabel(phase: WorldTime["phase"]): string {
  switch (phase) {
    case "late_night":
      return "глубокая ночь";
    case "dawn":
      return "рассвет";
    case "morning":
      return "утро";
    case "day":
      return "день";
    case "afternoon":
      return "после полудня";
    case "dusk":
      return "сумерки";
    case "evening":
      return "вечер";
    case "night":
      return "ночь";
  }
}

function mergeActor(current: Actor, patch: Parameters<typeof normalizeActor>[0]): Actor {
  const next = normalizeActor(patch, current.world_id);
  return {
    ...next,
    inventory_id: next.inventory_id || current.inventory_id,
  };
}

function advanceWorldTime(current: WorldTime, realSeconds: number): WorldTime {
  const rate = current.world_seconds_per_real_second || 1;
  const dayLength = current.day_length_seconds || 480;
  const nextWorldTime = current.world_time + realSeconds * rate;
  const dayOffset = ((nextWorldTime % dayLength) + dayLength) % dayLength;
  const phaseIndex = Math.min(
    DAY_PHASES.length - 1,
    Math.floor((dayOffset * DAY_PHASES.length) / dayLength),
  );
  const phaseStart = (phaseIndex * dayLength) / DAY_PHASES.length;
  const phaseEnd = ((phaseIndex + 1) * dayLength) / DAY_PHASES.length;
  const phaseLength = Math.max(phaseEnd - phaseStart, 1);

  return {
    ...current,
    world_time: nextWorldTime,
    day: Math.floor(nextWorldTime / dayLength) + 1,
    phase: DAY_PHASES[phaseIndex],
    phase_progress: (dayOffset - phaseStart) / phaseLength,
    day_progress: dayOffset / dayLength,
  };
}

function normalizeWorldTime(value: WorldTime): WorldTime {
  return {
    ...DEFAULT_WORLD_TIME,
    ...value,
    day_length_seconds:
      value.day_length_seconds || DEFAULT_WORLD_TIME.day_length_seconds,
    world_seconds_per_real_second:
      value.world_seconds_per_real_second ||
      DEFAULT_WORLD_TIME.world_seconds_per_real_second,
  };
}
