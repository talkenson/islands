"use strict";
(function () {
    const CHUNK_SIZE = 32;
    const TILE_SIZE = 12;
    const WATER_NONE = 0;
    const WATER_SEA = 1;
    const WATER_RIVER = 2;
    const WATER_LAKE = 3;
    const SOIL_SAND = 3;
    const SOIL_ROCKY = 8;
    const COVER_BUSH = 2;
    const COVER_DRY_BUSH = 3;
    const COVER_BIRCH_FOREST = 4;
    const COVER_PINE_FOREST = 5;
    const COVER_MIXED_FOREST = 6;
    const COVER_FLAG_ROCK = 1;
    const COVER_FLAG_MOUNTAIN = 2;
    const BIOME_TEMPERATE_FOREST = 2;
    const DEFAULT_RENDER_CONFIG = {
        seed: "demo:color",
        palette: {
            water: {
                sea: "#204063",
                lake: "#204063",
                tidal_sea: "#255b94",
                tidal_lake: "#338ab9",
                river: "#3a92b8",
                river_variant: "#2883ad",
            },
            terrain: {
                rock: "#777b74",
                coastal_rock: "#8b8879",
                sand: "#d6bd75",
                mountain_light: "#a6aaa2",
                mountain_dark: "#6e736d",
            },
            cover: {
                birch_forest: "#2f6b35",
                pine_forest: "#234f2d",
                mixed_forest: "#2d4d3f",
                dry_bush: "#756f35",
                bush: "#756f35",
            },
            biomes: {
                "3": ["#5f984f", "#6fa85f", "#83ad64", "#7bb96b"],
                "5": ["#4f735f", "#5a806a", "#3f604f", "#63766a"],
                "6": ["#8f9f52", "#a6b866", "#b8c878", "#7f8f48"],
                "2": ["#6fa85f", "#7bb96b", "#5f984f", "#83ad64"],
            },
        },
    };
    class ValueNoise {
        seed;
        values = new Map();
        constructor(seed) {
            this.seed = seed;
        }
        noise2D(x, y) {
            const x0 = Math.floor(x);
            const y0 = Math.floor(y);
            const x1 = x0 + 1;
            const y1 = y0 + 1;
            const sx = fade(x - x0);
            const sy = fade(y - y0);
            const n0 = lerp(this.lattice(x0, y0), this.lattice(x1, y0), sx);
            const n1 = lerp(this.lattice(x0, y1), this.lattice(x1, y1), sx);
            return lerp(n0, n1, sy);
        }
        octaveNoise2D(x, y, octaves, persistence) {
            let amplitude = 1;
            let frequency = 1;
            let total = 0;
            let maxValue = 0;
            for (let i = 0; i < octaves; i++) {
                total += this.noise2D(x * frequency, y * frequency) * amplitude;
                maxValue += amplitude;
                amplitude *= persistence;
                frequency *= 2;
            }
            return total / maxValue;
        }
        lattice(ix, iy) {
            const cacheKey = `${ix},${iy}`;
            const cached = this.values.get(cacheKey);
            if (cached !== undefined) {
                return cached;
            }
            const value = new Random(makeSeed(this.seed, cacheKey)).next();
            this.values.set(cacheKey, value);
            return value;
        }
    }
    class Random {
        state;
        constructor(seed) {
            this.state = hashSeed(seed)();
        }
        next() {
            this.state = (this.state + 0x6d2b79f5) >>> 0;
            let t = this.state;
            t = Math.imul((t ^ (t >>> 15)) >>> 0, (t | 1) >>> 0) >>> 0;
            t = (t ^ (t + Math.imul((t ^ (t >>> 7)) >>> 0, (t | 61) >>> 0))) >>> 0;
            return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
        }
    }
    function hashSeed(seed) {
        let h = (1779033703 ^ [...seed].length) >>> 0;
        for (const char of seed) {
            h = Math.imul((h ^ (char.codePointAt(0) || 0)) >>> 0, 3432918353) >>> 0;
            h = rotateLeft32(h, 13);
        }
        return () => {
            h = Math.imul((h ^ (h >>> 16)) >>> 0, 2246822507) >>> 0;
            h = Math.imul((h ^ (h >>> 13)) >>> 0, 3266489909) >>> 0;
            h = (h ^ (h >>> 16)) >>> 0;
            return h;
        };
    }
    function rotateLeft32(value, shift) {
        return ((value << shift) | (value >>> (32 - shift))) >>> 0;
    }
    function fade(t) {
        return t * t * t * (t * (t * 6 - 15) + 10);
    }
    function lerp(a, b, t) {
        return a + (b - a) * t;
    }
    function makeSeed(seed, salt) {
        return `${seed}:${salt}`;
    }
    const state = {
        token: "",
        userID: 1,
        actor: { id: 1, world_id: 1, x: 900, y: 1900 },
        worldID: 1,
        chunks: new Map(),
        lastEventID: 0,
        busy: false,
        streamAbort: null,
        renderConfig: DEFAULT_RENDER_CONFIG,
        colorNoise: new ValueNoise(DEFAULT_RENDER_CONFIG.seed),
        viewport: { scale: 1, ox: 0, oy: 0 },
    };
    const canvas = element("worldCanvas");
    const context = canvas.getContext("2d");
    if (!context) {
        throw new Error("2d canvas context is unavailable");
    }
    const ctx = context;
    const statusBox = element("status");
    const statusText = element("statusText");
    const emptyState = element("emptyState");
    const actorPosition = element("actorPosition");
    const chunkCount = element("chunkCount");
    const cellStock = element("cellStock");
    const eventLog = element("eventLog");
    const buttons = {
        up: element("moveUp"),
        down: element("moveDown"),
        left: element("moveLeft"),
        right: element("moveRight"),
        harvest: element("harvest"),
        reconnect: element("reconnect"),
    };
    function element(id) {
        const node = document.getElementById(id);
        if (!node) {
            throw new Error(`missing element #${id}`);
        }
        return node;
    }
    function key(cx, cy) {
        return `${cx},${cy}`;
    }
    function setStatus(kind, text) {
        statusBox.classList.toggle("live", kind === "live");
        statusBox.classList.toggle("error", kind === "error");
        statusText.textContent = text;
    }
    function addEvent(title, detail) {
        const item = document.createElement("li");
        item.innerHTML = `<b>${escapeHTML(title)}</b>${detail ? ` ${escapeHTML(detail)}` : ""}`;
        eventLog.prepend(item);
        while (eventLog.children.length > 40) {
            eventLog.lastElementChild?.remove();
        }
    }
    function escapeHTML(value) {
        return String(value).replace(/[&<>"']/g, (char) => {
            const replacements = {
                "&": "&amp;",
                "<": "&lt;",
                ">": "&gt;",
                "\"": "&quot;",
                "'": "&#39;",
            };
            return replacements[char] ?? char;
        });
    }
    function normalizeActor(actor) {
        return {
            id: actor.ID || actor.id || 1,
            world_id: actor.WorldID || actor.world_id || state.worldID,
            x: actor.X ?? actor.x ?? 0,
            y: actor.Y ?? actor.y ?? 0,
        };
    }
    async function login() {
        setStatus("idle", "вход");
        const res = await fetch("/api/v1/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ user_id: 1, actor_id: 1, world_id: 1 }),
        });
        if (!res.ok) {
            throw new Error(`login failed: ${res.status}`);
        }
        const data = await res.json();
        state.token = data.token;
        state.worldID = data.worlds[0]?.id || 1;
        if (data.actors[0]) {
            state.actor = normalizeActor(data.actors[0]);
        }
        addEvent("Вход выполнен", `world=${state.worldID}`);
    }
    async function connectStream() {
        if (state.streamAbort) {
            state.streamAbort.abort();
        }
        state.streamAbort = new AbortController();
        setStatus("idle", "подключение");
        const headers = { Authorization: `Bearer ${state.token}` };
        if (state.lastEventID > 0) {
            headers["Last-Event-ID"] = String(state.lastEventID);
        }
        const res = await fetch(`/api/v1/worlds/${state.worldID}/stream`, {
            headers,
            signal: state.streamAbort.signal,
        });
        if (!res.ok || !res.body) {
            throw new Error(`stream failed: ${res.status}`);
        }
        setStatus("live", "онлайн");
        await readSSE(res.body);
    }
    async function readSSE(body) {
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
                handleSSE(raw);
                splitAt = buffer.indexOf("\n\n");
            }
        }
    }
    function handleSSE(raw) {
        const lines = raw.split(/\r?\n/);
        let id = 0;
        let event = "message";
        const data = [];
        for (const line of lines) {
            if (line.startsWith("id:"))
                id = Number(line.slice(3).trim());
            if (line.startsWith("event:"))
                event = line.slice(6).trim();
            if (line.startsWith("data:"))
                data.push(line.slice(5).trimStart());
        }
        if (id > 0) {
            state.lastEventID = id;
        }
        if (data.length === 0) {
            return;
        }
        const payload = JSON.parse(data.join("\n"));
        applyEvent(payload.type || event, payload.data, payload.id || id);
    }
    function applyEvent(type, data, id) {
        if (type === "hello") {
            const hello = data;
            if (hello.render_config) {
                state.renderConfig = hello.render_config;
                state.colorNoise = new ValueNoise(hello.render_config.seed);
            }
            if (hello.actor) {
                state.actor = normalizeActor(hello.actor);
                updateUI();
                draw();
            }
            addEvent("Стрим открыт", `actor=${hello.actor_id}`);
            return;
        }
        if (type === "chunk_snapshot") {
            const chunk = normalizeChunk(data);
            state.chunks.set(key(chunk.cx, chunk.cy), chunk);
            emptyState.classList.add("hidden");
            updateUI();
            draw();
            return;
        }
        if (type === "entity_patch") {
            const patch = data;
            if (patch?.actor) {
                state.actor = normalizeActor(patch.actor);
                addEvent("Перемещение", `x=${state.actor.x}, y=${state.actor.y}, event=${id || patch.event_id || 0}`);
            }
            updateUI();
            draw();
        }
    }
    function normalizeChunk(data) {
        return {
            cx: data.cx,
            cy: data.cy,
            base: data.base || [],
            water: decodeByteLayer(data.water),
            cover: data.cover || [],
            stock: data.stock || [],
            meta: decodeByteLayer(data.meta),
            updatedTick: data.updated_tick || 0,
        };
    }
    function decodeByteLayer(value) {
        if (!value) {
            return new Uint8Array();
        }
        if (Array.isArray(value)) {
            return Uint8Array.from(value);
        }
        const binary = atob(value);
        const bytes = new Uint8Array(binary.length);
        for (let i = 0; i < binary.length; i++) {
            bytes[i] = binary.charCodeAt(i);
        }
        return bytes;
    }
    async function action(actionType, patch) {
        if (state.busy) {
            return;
        }
        state.busy = true;
        setBusy(true);
        try {
            const res = await fetch(`/api/v1/worlds/${state.worldID}/actions`, {
                method: "POST",
                headers: {
                    Authorization: `Bearer ${state.token}`,
                    "Content-Type": "application/json",
                },
                body: JSON.stringify({
                    action_type: actionType,
                    client_action_id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
                    ...patch,
                }),
            });
            const data = await res.json();
            if (!res.ok) {
                addEvent("Ошибка", data.message || data.code || res.status);
                return;
            }
            if (data.actor) {
                state.actor = normalizeActor(data.actor);
            }
            if (actionType === "harvest") {
                addEvent("Сбор", `event=${data.event_id || 0}`);
            }
            updateUI();
            draw();
        }
        finally {
            state.busy = false;
            setBusy(false);
        }
    }
    function setBusy(value) {
        for (const button of Object.values(buttons)) {
            button.disabled = value;
        }
    }
    function move(dx, dy) {
        void action("move", { x: state.actor.x + dx, y: state.actor.y + dy });
    }
    function actorChunkAndIndex() {
        const cx = Math.floor(state.actor.x / CHUNK_SIZE);
        const cy = Math.floor(state.actor.y / CHUNK_SIZE);
        const lx = ((state.actor.x % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
        const ly = ((state.actor.y % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
        return { cx, cy, index: ly * CHUNK_SIZE + lx };
    }
    function updateUI() {
        actorPosition.textContent = `${state.actor.x}, ${state.actor.y}`;
        chunkCount.textContent = String(state.chunks.size);
        const pos = actorChunkAndIndex();
        const chunk = state.chunks.get(key(pos.cx, pos.cy));
        cellStock.textContent = chunk ? String(chunk.stock[pos.index] || 0) : "-";
    }
    function resizeCanvas() {
        const rect = canvas.getBoundingClientRect();
        const ratio = window.devicePixelRatio || 1;
        canvas.width = Math.max(1, Math.floor(rect.width * ratio));
        canvas.height = Math.max(1, Math.floor(rect.height * ratio));
        draw();
    }
    function draw() {
        const width = canvas.width;
        const height = canvas.height;
        ctx.clearRect(0, 0, width, height);
        const ratio = window.devicePixelRatio || 1;
        const tile = TILE_SIZE * ratio;
        const centerX = width / 2 - (state.actor.x + 0.5) * tile;
        const centerY = height / 2 - (state.actor.y + 0.5) * tile;
        state.viewport = { scale: tile, ox: centerX, oy: centerY };
        ctx.fillStyle = "#0a1115";
        ctx.fillRect(0, 0, width, height);
        for (const chunk of state.chunks.values()) {
            drawChunk(chunk, tile, centerX, centerY);
        }
        drawGrid(tile, centerX, centerY);
        drawActor(tile, centerX, centerY);
    }
    function drawChunk(chunk, tile, ox, oy) {
        for (let i = 0; i < CHUNK_SIZE * CHUNK_SIZE; i++) {
            const lx = i % CHUNK_SIZE;
            const ly = Math.floor(i / CHUNK_SIZE);
            const wx = chunk.cx * CHUNK_SIZE + lx;
            const wy = chunk.cy * CHUNK_SIZE + ly;
            const x = Math.floor(ox + wx * tile);
            const y = Math.floor(oy + wy * tile);
            if (x + tile < 0 || y + tile < 0 || x > canvas.width || y > canvas.height) {
                continue;
            }
            ctx.fillStyle = cellColor(chunk, i, wx, wy);
            ctx.fillRect(x, y, Math.ceil(tile), Math.ceil(tile));
        }
    }
    function cellColor(chunk, index, wx, wy) {
        const palette = state.renderConfig.palette;
        const base = chunk.base[index] || 0;
        const water = chunk.water[index] || 0;
        const cover = chunk.cover[index] || 0;
        const stock = chunk.stock[index] || 0;
        const soil = (base >> 5) & 15;
        const biome = base & 31;
        const elevation = (base >> 9) & 31;
        const waterKind = water & 15;
        const waterLevel = (water >> 4) & 7;
        const waterTidal = (water & 128) !== 0;
        const coverKind = cover & 255;
        const coverLevel = (cover >> 8) & 15;
        const coverFlags = (cover >> 12) & 15;
        const height = chunk.meta.length > index ? (chunk.meta[index] || 0) / 255 : elevation / 31;
        if (waterKind !== WATER_NONE) {
            if (waterKind === WATER_RIVER) {
                const baseColor = state.colorNoise.noise2D(wx * 0.2, wy * 0.2) > 0.55
                    ? palette.water.river_variant
                    : palette.water.river;
                return shade(baseColor, Math.round((waterLevel - 1) * 4));
            }
            if (waterTidal) {
                const baseColor = waterKind === WATER_LAKE ? palette.water.tidal_lake : palette.water.tidal_sea;
                return shade(baseColor, Math.round(waterLevel * -1.5 + height * 12));
            }
            if (waterKind === WATER_LAKE) {
                return shade(palette.water.lake, Math.round(height * 18));
            }
            return shade(palette.water.sea, Math.round(height * 22));
        }
        if ((coverFlags & COVER_FLAG_MOUNTAIN) !== 0) {
            const n = state.colorNoise.octaveNoise2D(wx * 0.13 + 11, wy * 0.13 + 19, 3, 0.5);
            const baseColor = n <= 0.58 ? palette.terrain.mountain_dark : palette.terrain.mountain_light;
            return shade(baseColor, Math.round((n - 0.5) * 22));
        }
        if ((coverFlags & COVER_FLAG_ROCK) !== 0 || soil === SOIL_ROCKY) {
            const n = state.colorNoise.octaveNoise2D(wx * 0.13 + 11, wy * 0.13 + 19, 3, 0.5);
            if (hasWaterNeighbor(wx, wy)) {
                return shade(palette.terrain.coastal_rock, Math.round((n - 0.5) * 18));
            }
            return shade(palette.terrain.rock, Math.round((n - 0.5) * 20));
        }
        switch (coverKind) {
            case COVER_BIRCH_FOREST:
                return shade(palette.cover.birch_forest, coverLevel * 2 + Math.round(coverDensity(stock) * 12));
            case COVER_PINE_FOREST:
                return shade(palette.cover.pine_forest, coverLevel * 2 + Math.round(coverDensity(stock) * 12));
            case COVER_MIXED_FOREST:
                return shade(palette.cover.mixed_forest, coverLevel * 2 + Math.round(coverDensity(stock) * 12));
            case COVER_DRY_BUSH:
                return shade(palette.cover.dry_bush, Math.round(coverDensity(stock) * 10));
            case COVER_BUSH:
                return shade(palette.cover.bush, coverLevel);
        }
        if (soil === SOIL_SAND) {
            const n = state.colorNoise.octaveNoise2D(wx * 0.11 + 7, wy * 0.11 + 13, 3, 0.52);
            return shade(palette.terrain.sand, Math.round((n - 0.5) * 16));
        }
        const colors = biomeColors(biome);
        const n = state.colorNoise.octaveNoise2D(wx * 0.09, wy * 0.09, 3, 0.5);
        const colorIndex = Math.min(colors.length - 1, Math.floor(n * colors.length));
        return shade(colors[colorIndex] || colors[0] || "#7bb96b", Math.round((n - 0.5) * 18));
    }
    function coverDensity(stock) {
        return clamp((stock - 6) / 18, 0.28, 1);
    }
    function biomeColors(biome) {
        return state.renderConfig.palette.biomes[String(biome)]
            || state.renderConfig.palette.biomes[String(BIOME_TEMPERATE_FOREST)]
            || DEFAULT_RENDER_CONFIG.palette.biomes[String(BIOME_TEMPERATE_FOREST)];
    }
    function hasWaterNeighbor(wx, wy) {
        return waterKindAt(wx - 1, wy) !== WATER_NONE
            || waterKindAt(wx + 1, wy) !== WATER_NONE
            || waterKindAt(wx, wy - 1) !== WATER_NONE
            || waterKindAt(wx, wy + 1) !== WATER_NONE;
    }
    function waterKindAt(wx, wy) {
        const cx = Math.floor(wx / CHUNK_SIZE);
        const cy = Math.floor(wy / CHUNK_SIZE);
        const lx = ((wx % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
        const ly = ((wy % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
        const chunk = state.chunks.get(key(cx, cy));
        if (!chunk) {
            return WATER_NONE;
        }
        return (chunk.water[ly * CHUNK_SIZE + lx] || 0) & 15;
    }
    function shade(hex, amount) {
        const value = hex.slice(1);
        const parts = [0, 2, 4].map((i) => parseInt(value.slice(i, i + 2), 16));
        const channels = parts.map((part) => Math.max(0, Math.min(255, Math.round(part + amount))));
        return `rgb(${channels.join(",")})`;
    }
    function clamp(value, min, max) {
        return Math.max(min, Math.min(max, value));
    }
    function drawGrid(tile, ox, oy) {
        if (tile < 7) {
            return;
        }
        const startX = Math.floor((-ox) / tile) - 1;
        const endX = Math.ceil((canvas.width - ox) / tile) + 1;
        const startY = Math.floor((-oy) / tile) - 1;
        const endY = Math.ceil((canvas.height - oy) / tile) + 1;
        ctx.strokeStyle = "rgba(0, 0, 0, 0.18)";
        ctx.lineWidth = 1;
        ctx.beginPath();
        for (let x = startX; x <= endX; x++) {
            const px = Math.floor(ox + x * tile) + 0.5;
            ctx.moveTo(px, 0);
            ctx.lineTo(px, canvas.height);
        }
        for (let y = startY; y <= endY; y++) {
            const py = Math.floor(oy + y * tile) + 0.5;
            ctx.moveTo(0, py);
            ctx.lineTo(canvas.width, py);
        }
        ctx.stroke();
    }
    function drawActor(tile, ox, oy) {
        const x = ox + state.actor.x * tile;
        const y = oy + state.actor.y * tile;
        ctx.fillStyle = "#ffe8a0";
        ctx.strokeStyle = "#18120a";
        ctx.lineWidth = Math.max(2, tile * 0.15);
        ctx.beginPath();
        ctx.arc(x + tile / 2, y + tile / 2, tile * 0.45, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
    }
    function startStream() {
        connectStream().catch((err) => {
            if (!isAbortError(err)) {
                setStatus("error", "стрим упал");
                addEvent("Стрим", errorMessage(err));
            }
        });
    }
    function isAbortError(err) {
        return err instanceof DOMException && err.name === "AbortError";
    }
    function errorMessage(err) {
        return err instanceof Error ? err.message : String(err);
    }
    async function boot() {
        try {
            await login();
            updateUI();
            resizeCanvas();
            startStream();
        }
        catch (err) {
            setStatus("error", "ошибка входа");
            addEvent("Ошибка", errorMessage(err));
        }
    }
    buttons.up.addEventListener("click", () => move(0, -1));
    buttons.down.addEventListener("click", () => move(0, 1));
    buttons.left.addEventListener("click", () => move(-1, 0));
    buttons.right.addEventListener("click", () => move(1, 0));
    buttons.harvest.addEventListener("click", () => void action("harvest", {}));
    buttons.reconnect.addEventListener("click", startStream);
    window.addEventListener("resize", resizeCanvas);
    window.addEventListener("keydown", (event) => {
        if (event.repeat || state.busy) {
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
    });
    void boot();
})();
