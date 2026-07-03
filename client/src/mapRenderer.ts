import {
  Application,
  CanvasSource,
  Container,
  Graphics,
  Sprite,
  Texture,
} from "pixi.js";
import { CHUNK_SIZE, DEFAULT_RENDER_CONFIG, TILE_SIZE } from "./config";
import type { ColorNoise } from "./noise";
import { ValueNoise } from "./noise";
import { chunkKey } from "./chunks";
import type {
  Actor,
  ChunkSnapshot,
  RenderConfig,
  Viewport,
  WorldCell,
} from "./types";

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
const FOG_CLEAR_TILES = CHUNK_SIZE * 0.8;
const FOG_FULL_TILES = CHUNK_SIZE * 1.75;
const FOG_MAX_ALPHA = 1;
const FOG_RGB = [7, 11, 13] as const;
const FOG_MASK_BASE_SCALE = 0.18;
const FOG_MASK_MAX_WIDTH = 320;
const FOG_MASK_MAX_HEIGHT = 240;
const FOG_LOADED_CHUNK_MULTIPLIER = 0.8;
const FOG_LOADED_EDGE_FEATHER_TILES = 8;
type ChunkLookup = Map<number, Set<number>>;

interface ChunkView {
  sprite: Sprite;
  texture: Texture;
  source: ChunkSnapshot;
  renderRevision: number;
}

interface PendingFrame {
  actor: Actor;
  chunks: Map<string, ChunkSnapshot>;
}

export class MapRenderer {
  private readonly fogCanvas: HTMLCanvasElement;
  private readonly fogCtx: CanvasRenderingContext2D;
  private fogTexture: Texture;
  private readonly fogSprite: Sprite;
  private readonly worldLayer = new Container();
  private readonly chunkLayer = new Container();
  private readonly gridLayer = new Graphics({ roundPixels: true });
  private readonly actorLayer = new Graphics({ roundPixels: true });
  private readonly chunkViews = new Map<string, ChunkView>();
  private app: Application | undefined;
  private initialized = false;
  private destroyed = false;
  private pendingFrame: PendingFrame | undefined;
  private fogImage: ImageData | undefined;
  private renderConfig = DEFAULT_RENDER_CONFIG;
  private colorNoise: ColorNoise = new ValueNoise(DEFAULT_RENDER_CONFIG.seed);
  private textureRevision = 0;
  private viewport: Viewport = { scale: 1, ox: 0, oy: 0, zoom: 2 };

  constructor(private readonly canvas: HTMLCanvasElement) {
    this.fogCanvas = document.createElement("canvas");
    this.fogCanvas.width = 1;
    this.fogCanvas.height = 1;
    const fogContext = this.fogCanvas.getContext("2d");
    if (!fogContext) {
      throw new Error("2d fog canvas context is unavailable");
    }
    this.fogCtx = fogContext;
    this.fogTexture = this.textureFromCanvas(this.fogCanvas, "linear");
    this.fogSprite = new Sprite({ texture: this.fogTexture });

    void this.init();
  }

  setRenderConfig(renderConfig: RenderConfig): void {
    this.renderConfig = renderConfig;
    this.colorNoise = new ValueNoise(renderConfig.seed);
    this.textureRevision += 1;
    if (this.pendingFrame) {
      return;
    }
    if (this.initialized && this.app) {
      this.app.render();
    }
  }

  resize(actor: Actor, chunks: Map<string, ChunkSnapshot>): void {
    const width = Math.max(1, Math.floor(window.innerWidth));
    const height = Math.max(1, Math.floor(window.innerHeight));
    if (!this.initialized || !this.app) {
      this.pendingFrame = { actor, chunks };
      return;
    }
    this.app.renderer.resize(width, height);
    this.draw(actor, chunks);
  }

  zoom(
    deltaY: number,
    minZoom: number,
    maxZoom: number,
    actor: Actor,
    chunks: Map<string, ChunkSnapshot>,
  ): void {
    const direction = deltaY > 0 ? -1 : 1;
    const factor = direction > 0 ? 1.16 : 1 / 1.16;
    const nextZoom = clamp(this.viewport.zoom * factor, minZoom, maxZoom);
    if (nextZoom === this.viewport.zoom) {
      return;
    }
    this.viewport.zoom = nextZoom;
    this.draw(actor, chunks);
  }

  cellAtClientPoint(clientX: number, clientY: number): WorldCell | undefined {
    const rect = this.canvas.getBoundingClientRect();
    if (
      clientX < rect.left ||
      clientY < rect.top ||
      clientX > rect.right ||
      clientY > rect.bottom
    ) {
      return undefined;
    }

    const px = clientX - rect.left;
    const py = clientY - rect.top;
    return {
      x: Math.floor((px - this.viewport.ox) / this.viewport.scale),
      y: Math.floor((py - this.viewport.oy) / this.viewport.scale),
    };
  }

  draw(actor: Actor, chunks: Map<string, ChunkSnapshot>): void {
    if (!this.initialized || !this.app) {
      this.pendingFrame = { actor, chunks };
      return;
    }

    const width = this.app.screen.width;
    const height = this.app.screen.height;
    const tile = TILE_SIZE * this.viewport.zoom;
    const centerX = width / 2 - (actor.x + 0.5) * tile;
    const centerY = height / 2 - (actor.y + 0.5) * tile;
    this.viewport = {
      scale: tile,
      ox: centerX,
      oy: centerY,
      zoom: this.viewport.zoom,
    };

    this.updateChunkSprites(chunks);
    this.worldLayer.position.set(centerX, centerY);
    this.worldLayer.scale.set(tile);

    this.drawGrid(tile, centerX, centerY, width, height);
    this.drawFogOverlay(actor, chunks, tile, centerX, centerY, width, height);
    this.drawActor(actor, tile, centerX, centerY);
    this.app.render();
  }

  destroy(): void {
    this.destroyed = true;
    this.chunkViews.forEach((view) => {
      view.texture.destroy(true);
      view.sprite.destroy();
    });
    this.chunkViews.clear();
    this.fogTexture.destroy(true);
    this.app?.destroy(false, {
      children: true,
      texture: false,
      textureSource: false,
    });
    this.app = undefined;
  }

  private async init(): Promise<void> {
    const app = new Application();
    this.app = app;
    await app.init({
      canvas: this.canvas,
      width: Math.max(1, Math.floor(window.innerWidth)),
      height: Math.max(1, Math.floor(window.innerHeight)),
      resolution: window.devicePixelRatio || 1,
      autoDensity: true,
      backgroundColor: 0x0a1115,
      preference: ["webgl", "canvas"],
      antialias: false,
      autoStart: false,
      roundPixels: true,
    });
    if (this.destroyed) {
      app.destroy(false);
      return;
    }

    app.stage.addChild(this.worldLayer);
    this.worldLayer.addChild(this.chunkLayer);
    app.stage.addChild(this.gridLayer);
    app.stage.addChild(this.fogSprite);
    app.stage.addChild(this.actorLayer);
    this.initialized = true;

    if (this.pendingFrame) {
      const frame = this.pendingFrame;
      this.pendingFrame = undefined;
      this.draw(frame.actor, frame.chunks);
    } else {
      app.render();
    }
  }

  private updateChunkSprites(chunks: Map<string, ChunkSnapshot>): void {
    const dirty = new Set<string>();

    for (const [key, chunk] of chunks) {
      const view = this.chunkViews.get(key);
      if (
        !view ||
        view.source !== chunk ||
        view.renderRevision !== this.textureRevision
      ) {
        addDirtyChunkAndNeighbors(dirty, chunk.cx, chunk.cy);
      }
    }

    for (const key of this.chunkViews.keys()) {
      if (!chunks.has(key)) {
        const view = this.chunkViews.get(key);
        if (view) {
          this.chunkLayer.removeChild(view.sprite);
          view.texture.destroy(true);
          view.sprite.destroy();
          this.chunkViews.delete(key);
        }
      }
    }

    for (const key of dirty) {
      const chunk = chunks.get(key);
      if (!chunk) {
        continue;
      }
      this.upsertChunkSprite(key, chunk, chunks);
    }
  }

  private upsertChunkSprite(
    key: string,
    chunk: ChunkSnapshot,
    chunks: Map<string, ChunkSnapshot>,
  ): void {
    const texture = this.buildChunkTexture(chunk, chunks);
    const view = this.chunkViews.get(key);
    if (!view) {
      const sprite = new Sprite({ texture, roundPixels: true });
      sprite.position.set(chunk.cx * CHUNK_SIZE, chunk.cy * CHUNK_SIZE);
      this.chunkLayer.addChild(sprite);
      this.chunkViews.set(key, {
        sprite,
        texture,
        source: chunk,
        renderRevision: this.textureRevision,
      });
      return;
    }

    const oldTexture = view.texture;
    view.sprite.texture = texture;
    view.sprite.position.set(chunk.cx * CHUNK_SIZE, chunk.cy * CHUNK_SIZE);
    view.texture = texture;
    view.source = chunk;
    view.renderRevision = this.textureRevision;
    oldTexture.destroy(true);
  }

  private buildChunkTexture(
    chunk: ChunkSnapshot,
    chunks: Map<string, ChunkSnapshot>,
  ): Texture {
    const canvas = document.createElement("canvas");
    canvas.width = CHUNK_SIZE;
    canvas.height = CHUNK_SIZE;
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("2d chunk canvas context is unavailable");
    }
    context.imageSmoothingEnabled = false;
    for (let i = 0; i < CHUNK_SIZE * CHUNK_SIZE; i++) {
      const lx = i % CHUNK_SIZE;
      const ly = Math.floor(i / CHUNK_SIZE);
      const wx = chunk.cx * CHUNK_SIZE + lx;
      const wy = chunk.cy * CHUNK_SIZE + ly;
      context.fillStyle = this.cellColor(chunk, chunks, i, wx, wy);
      context.fillRect(lx, ly, 1, 1);
    }
    return this.textureFromCanvas(canvas, "nearest");
  }

  private textureFromCanvas(
    canvas: HTMLCanvasElement,
    scaleMode: "nearest" | "linear",
  ): Texture {
    const source = new CanvasSource({
      resource: canvas,
      scaleMode,
      autoGenerateMipmaps: false,
    });
    return new Texture({ source });
  }

  private drawFogOverlay(
    actor: Actor,
    chunks: Map<string, ChunkSnapshot>,
    tile: number,
    ox: number,
    oy: number,
    screenWidth: number,
    screenHeight: number,
  ): void {
    const scale = Math.min(
      FOG_MASK_BASE_SCALE,
      FOG_MASK_MAX_WIDTH / screenWidth,
      FOG_MASK_MAX_HEIGHT / screenHeight,
    );
    const width = Math.max(1, Math.ceil(screenWidth * scale));
    const height = Math.max(1, Math.ceil(screenHeight * scale));
    if (this.fogCanvas.width !== width || this.fogCanvas.height !== height) {
      this.fogCanvas.width = width;
      this.fogCanvas.height = height;
      this.fogImage = undefined;
      this.replaceFogTexture();
    }

    const image =
      this.fogImage &&
      this.fogImage.width === width &&
      this.fogImage.height === height
        ? this.fogImage
        : this.fogCtx.createImageData(width, height);
    this.fogImage = image;
    const actorX = actor.x + 0.5;
    const actorY = actor.y + 0.5;
    const data = image.data;
    const chunkLookup = buildChunkLookup(chunks);

    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        const screenX = (x + 0.5) / scale;
        const screenY = (y + 0.5) / scale;
        const wx = (screenX - ox) / tile;
        const wy = (screenY - oy) / tile;
        const alpha =
          this.fogAlpha(wx - actorX, wy - actorY) *
          this.loadedChunkFogMultiplier(chunkLookup, wx, wy);
        const offset = (y * width + x) * 4;
        data[offset] = FOG_RGB[0];
        data[offset + 1] = FOG_RGB[1];
        data[offset + 2] = FOG_RGB[2];
        data[offset + 3] = Math.round(alpha * 255);
      }
    }
    this.fogCtx.putImageData(image, 0, 0);
    this.fogTexture.source.update();
    this.fogSprite.position.set(0, 0);
    this.fogSprite.width = screenWidth;
    this.fogSprite.height = screenHeight;
  }

  private replaceFogTexture(): void {
    const oldTexture = this.fogTexture;
    this.fogTexture = this.textureFromCanvas(this.fogCanvas, "linear");
    this.fogSprite.texture = this.fogTexture;
    oldTexture.destroy(true);
  }

  private loadedChunkFogMultiplier(
    chunks: ChunkLookup,
    wx: number,
    wy: number,
  ): number {
    const cx = Math.floor(wx / CHUNK_SIZE);
    const cy = Math.floor(wy / CHUNK_SIZE);
    if (!hasChunk(chunks, cx, cy)) {
      return 1;
    }

    const lx = wx - cx * CHUNK_SIZE;
    const ly = wy - cy * CHUNK_SIZE;
    let edgeDistance = FOG_LOADED_EDGE_FEATHER_TILES;
    if (!hasChunk(chunks, cx - 1, cy)) {
      edgeDistance = Math.min(edgeDistance, lx);
    }
    if (!hasChunk(chunks, cx + 1, cy)) {
      edgeDistance = Math.min(edgeDistance, CHUNK_SIZE - lx);
    }
    if (!hasChunk(chunks, cx, cy - 1)) {
      edgeDistance = Math.min(edgeDistance, ly);
    }
    if (!hasChunk(chunks, cx, cy + 1)) {
      edgeDistance = Math.min(edgeDistance, CHUNK_SIZE - ly);
    }

    const t = smoothstep(edgeDistance / FOG_LOADED_EDGE_FEATHER_TILES);
    return lerp(1, FOG_LOADED_CHUNK_MULTIPLIER, t);
  }

  private fogAlpha(dx: number, dy: number): number {
    const distance = roundedSquareDistance(dx, dy);
    const t = smoothstep(
      (distance - FOG_CLEAR_TILES) / (FOG_FULL_TILES - FOG_CLEAR_TILES),
    );
    return clamp(t * FOG_MAX_ALPHA, 0, FOG_MAX_ALPHA);
  }

  private cellColor(
    chunk: ChunkSnapshot,
    chunks: Map<string, ChunkSnapshot>,
    index: number,
    wx: number,
    wy: number,
  ): string {
    const palette = this.renderConfig.palette;
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
    const height =
      chunk.meta.length > index
        ? (chunk.meta[index] || 0) / 255
        : elevation / 31;

    if (waterKind !== WATER_NONE) {
      if (waterKind === WATER_RIVER) {
        const baseColor =
          this.colorNoise.noise2D(wx * 0.2, wy * 0.2) > 0.55
            ? palette.water.river_variant
            : palette.water.river;
        return shade(baseColor, Math.round((waterLevel - 1) * 4));
      }
      if (waterTidal) {
        const baseColor =
          waterKind === WATER_LAKE
            ? palette.water.tidal_lake
            : palette.water.tidal_sea;
        return shade(baseColor, Math.round(waterLevel * -1.5 + height * 12));
      }
      if (waterKind === WATER_LAKE) {
        return shade(palette.water.lake, Math.round(height * 18));
      }
      return shade(palette.water.sea, Math.round(height * 22));
    }

    if ((coverFlags & COVER_FLAG_MOUNTAIN) !== 0) {
      const n = this.colorNoise.octaveNoise2D(
        wx * 0.13 + 11,
        wy * 0.13 + 19,
        3,
        0.5,
      );
      const baseColor =
        n <= 0.58
          ? palette.terrain.mountain_dark
          : palette.terrain.mountain_light;
      return shade(baseColor, Math.round((n - 0.5) * 22));
    }
    if ((coverFlags & COVER_FLAG_ROCK) !== 0 || soil === SOIL_ROCKY) {
      const n = this.colorNoise.octaveNoise2D(
        wx * 0.13 + 11,
        wy * 0.13 + 19,
        3,
        0.5,
      );
      if (this.hasWaterNeighbor(chunks, wx, wy)) {
        return shade(palette.terrain.coastal_rock, Math.round((n - 0.5) * 18));
      }
      return shade(palette.terrain.rock, Math.round((n - 0.5) * 20));
    }

    switch (coverKind) {
      case COVER_BIRCH_FOREST:
        return shade(
          palette.cover.birch_forest,
          coverLevel * 2 + Math.round(coverDensity(stock) * 12),
        );
      case COVER_PINE_FOREST:
        return shade(
          palette.cover.pine_forest,
          coverLevel * 2 + Math.round(coverDensity(stock) * 12),
        );
      case COVER_MIXED_FOREST:
        return shade(
          palette.cover.mixed_forest,
          coverLevel * 2 + Math.round(coverDensity(stock) * 12),
        );
      case COVER_DRY_BUSH:
        return shade(
          palette.cover.dry_bush,
          Math.round(coverDensity(stock) * 10),
        );
      case COVER_BUSH:
        return shade(palette.cover.bush, coverLevel);
    }

    if (soil === SOIL_SAND) {
      const n = this.colorNoise.octaveNoise2D(
        wx * 0.11 + 7,
        wy * 0.11 + 13,
        3,
        0.52,
      );
      return shade(palette.terrain.sand, Math.round((n - 0.5) * 16));
    }

    const colors = this.biomeColors(biome);
    const n = this.colorNoise.octaveNoise2D(wx * 0.09, wy * 0.09, 3, 0.5);
    const colorIndex = Math.min(
      colors.length - 1,
      Math.floor(n * colors.length),
    );
    return shade(
      colors[colorIndex] || colors[0] || "#7bb96b",
      Math.round((n - 0.5) * 18),
    );
  }

  private biomeColors(biome: number): string[] {
    return (
      this.renderConfig.palette.biomes[String(biome)] ||
      this.renderConfig.palette.biomes[String(BIOME_TEMPERATE_FOREST)] ||
      DEFAULT_RENDER_CONFIG.palette.biomes[String(BIOME_TEMPERATE_FOREST)]
    );
  }

  private hasWaterNeighbor(
    chunks: Map<string, ChunkSnapshot>,
    wx: number,
    wy: number,
  ): boolean {
    return (
      this.waterKindAt(chunks, wx - 1, wy) !== WATER_NONE ||
      this.waterKindAt(chunks, wx + 1, wy) !== WATER_NONE ||
      this.waterKindAt(chunks, wx, wy - 1) !== WATER_NONE ||
      this.waterKindAt(chunks, wx, wy + 1) !== WATER_NONE
    );
  }

  private waterKindAt(
    chunks: Map<string, ChunkSnapshot>,
    wx: number,
    wy: number,
  ): number {
    const cx = Math.floor(wx / CHUNK_SIZE);
    const cy = Math.floor(wy / CHUNK_SIZE);
    const lx = ((wx % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
    const ly = ((wy % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
    const chunk = chunks.get(chunkKey(cx, cy));
    if (!chunk) {
      return WATER_NONE;
    }
    return (chunk.water[ly * CHUNK_SIZE + lx] || 0) & 15;
  }

  private drawGrid(
    tile: number,
    ox: number,
    oy: number,
    screenWidth: number,
    screenHeight: number,
  ): void {
    this.gridLayer.clear();
    if (tile < 7) {
      return;
    }
    const startX = Math.floor(-ox / tile) - 1;
    const endX = Math.ceil((screenWidth - ox) / tile) + 1;
    const startY = Math.floor(-oy / tile) - 1;
    const endY = Math.ceil((screenHeight - oy) / tile) + 1;

    for (let x = startX; x <= endX; x++) {
      const px = Math.floor(ox + x * tile);
      this.gridLayer.rect(px, 0, 1, screenHeight).fill({
        color: 0x000000,
        alpha: 0.18,
      });
    }
    for (let y = startY; y <= endY; y++) {
      const py = Math.floor(oy + y * tile);
      this.gridLayer.rect(0, py, screenWidth, 1).fill({
        color: 0x000000,
        alpha: 0.18,
      });
    }
  }

  private drawActor(actor: Actor, tile: number, ox: number, oy: number): void {
    const x = ox + actor.x * tile + tile / 2;
    const y = oy + actor.y * tile + tile / 2;
    this.actorLayer
      .clear()
      .circle(x, y, tile * 0.45)
      .fill({ color: 0xffe8a0 })
      .stroke({ color: 0x18120a, width: Math.max(2, tile * 0.15) });
  }
}

function addDirtyChunkAndNeighbors(
  dirty: Set<string>,
  cx: number,
  cy: number,
): void {
  dirty.add(chunkKey(cx, cy));
  dirty.add(chunkKey(cx - 1, cy));
  dirty.add(chunkKey(cx + 1, cy));
  dirty.add(chunkKey(cx, cy - 1));
  dirty.add(chunkKey(cx, cy + 1));
}

function coverDensity(stock: number): number {
  return clamp((stock - 6) / 18, 0.28, 1);
}

function shade(hex: string, amount: number): string {
  const value = hex.slice(1);
  const parts = [0, 2, 4].map((i) => parseInt(value.slice(i, i + 2), 16));
  const channels = parts.map((part) =>
    Math.max(0, Math.min(255, Math.round(part + amount))),
  );
  return `rgb(${channels.join(",")})`;
}

function roundedSquareDistance(dx: number, dy: number): number {
  const ax = Math.abs(dx);
  const ay = Math.abs(dy);
  const ax2 = ax * ax;
  const ay2 = ay * ay;
  return Math.sqrt(Math.sqrt(ax2 * ax2 + ay2 * ay2));
}

function buildChunkLookup(chunks: Map<string, ChunkSnapshot>): ChunkLookup {
  const lookup: ChunkLookup = new Map();
  for (const chunk of chunks.values()) {
    let column = lookup.get(chunk.cx);
    if (!column) {
      column = new Set();
      lookup.set(chunk.cx, column);
    }
    column.add(chunk.cy);
  }
  return lookup;
}

function hasChunk(chunks: ChunkLookup, cx: number, cy: number): boolean {
  return chunks.get(cx)?.has(cy) || false;
}

function smoothstep(value: number): number {
  const t = clamp(value, 0, 1);
  return t * t * (3 - 2 * t);
}

function lerp(start: number, end: number, t: number): number {
  return start + (end - start) * t;
}

export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}
