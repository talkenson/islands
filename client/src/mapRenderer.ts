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
const FOG_MASK_SCALE = 0.4;
const FOG_SHAPE_POWER = 4;

export class MapRenderer {
  private readonly ctx: CanvasRenderingContext2D;
  private readonly fogCanvas: HTMLCanvasElement;
  private readonly fogCtx: CanvasRenderingContext2D;
  private renderConfig = DEFAULT_RENDER_CONFIG;
  private colorNoise: ColorNoise = new ValueNoise(DEFAULT_RENDER_CONFIG.seed);
  private viewport: Viewport = { scale: 1, ox: 0, oy: 0, zoom: 2 };

  constructor(private readonly canvas: HTMLCanvasElement) {
    const context = canvas.getContext("2d");
    if (!context) {
      throw new Error("2d canvas context is unavailable");
    }
    this.ctx = context;
    this.fogCanvas = document.createElement("canvas");
    const fogContext = this.fogCanvas.getContext("2d");
    if (!fogContext) {
      throw new Error("2d fog canvas context is unavailable");
    }
    this.fogCtx = fogContext;
  }

  setRenderConfig(renderConfig: RenderConfig): void {
    this.renderConfig = renderConfig;
    this.colorNoise = new ValueNoise(renderConfig.seed);
  }

  resize(actor: Actor, chunks: Map<string, ChunkSnapshot>): void {
    const ratio = window.devicePixelRatio || 1;
    this.canvas.width = Math.max(1, Math.floor(window.innerWidth * ratio));
    this.canvas.height = Math.max(1, Math.floor(window.innerHeight * ratio));
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

    const px = (clientX - rect.left) * (this.canvas.width / rect.width);
    const py = (clientY - rect.top) * (this.canvas.height / rect.height);
    return {
      x: Math.floor((px - this.viewport.ox) / this.viewport.scale),
      y: Math.floor((py - this.viewport.oy) / this.viewport.scale),
    };
  }

  draw(actor: Actor, chunks: Map<string, ChunkSnapshot>): void {
    const width = this.canvas.width;
    const height = this.canvas.height;
    this.ctx.clearRect(0, 0, width, height);

    const ratio = window.devicePixelRatio || 1;
    const tile = TILE_SIZE * this.viewport.zoom * ratio;
    const centerX = width / 2 - (actor.x + 0.5) * tile;
    const centerY = height / 2 - (actor.y + 0.5) * tile;
    this.viewport = {
      scale: tile,
      ox: centerX,
      oy: centerY,
      zoom: this.viewport.zoom,
    };

    this.ctx.fillStyle = "#0a1115";
    this.ctx.fillRect(0, 0, width, height);

    for (const chunk of chunks.values()) {
      this.drawChunk(chunk, chunks, tile, centerX, centerY);
    }

    this.drawGrid(tile, centerX, centerY);
    this.drawFogOverlay(actor, tile, centerX, centerY);
    this.drawActor(actor, tile, centerX, centerY);
  }

  private drawChunk(
    chunk: ChunkSnapshot,
    chunks: Map<string, ChunkSnapshot>,
    tile: number,
    ox: number,
    oy: number,
  ): void {
    for (let i = 0; i < CHUNK_SIZE * CHUNK_SIZE; i++) {
      const lx = i % CHUNK_SIZE;
      const ly = Math.floor(i / CHUNK_SIZE);
      const wx = chunk.cx * CHUNK_SIZE + lx;
      const wy = chunk.cy * CHUNK_SIZE + ly;
      const x = Math.floor(ox + wx * tile);
      const y = Math.floor(oy + wy * tile);
      if (
        x + tile < 0 ||
        y + tile < 0 ||
        x > this.canvas.width ||
        y > this.canvas.height
      ) {
        continue;
      }
      this.ctx.fillStyle = this.cellColor(chunk, chunks, i, wx, wy);
      this.ctx.fillRect(x, y, Math.ceil(tile), Math.ceil(tile));
    }
  }

  private drawFogOverlay(actor: Actor, tile: number, ox: number, oy: number): void {
    const width = Math.max(1, Math.ceil(this.canvas.width * FOG_MASK_SCALE));
    const height = Math.max(1, Math.ceil(this.canvas.height * FOG_MASK_SCALE));
    if (this.fogCanvas.width !== width || this.fogCanvas.height !== height) {
      this.fogCanvas.width = width;
      this.fogCanvas.height = height;
    }

    const image = this.fogCtx.createImageData(width, height);
    const actorX = actor.x + 0.5;
    const actorY = actor.y + 0.5;

    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        const screenX = (x + 0.5) / FOG_MASK_SCALE;
        const screenY = (y + 0.5) / FOG_MASK_SCALE;
        const wx = (screenX - ox) / tile;
        const wy = (screenY - oy) / tile;
        const alpha = this.fogAlpha(wx - actorX, wy - actorY);
        const offset = (y * width + x) * 4;
        image.data[offset] = FOG_RGB[0];
        image.data[offset + 1] = FOG_RGB[1];
        image.data[offset + 2] = FOG_RGB[2];
        image.data[offset + 3] = Math.round(alpha * 255);
      }
    }
    this.fogCtx.putImageData(image, 0, 0);

    const previousSmoothing = this.ctx.imageSmoothingEnabled;
    this.ctx.imageSmoothingEnabled = true;
    this.ctx.drawImage(this.fogCanvas, 0, 0, this.canvas.width, this.canvas.height);
    this.ctx.imageSmoothingEnabled = previousSmoothing;
  }

  private fogAlpha(dx: number, dy: number): number {
    const distance = superellipseDistance(dx, dy, FOG_SHAPE_POWER);
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

  private drawGrid(tile: number, ox: number, oy: number): void {
    if (tile < 7) {
      return;
    }
    const startX = Math.floor(-ox / tile) - 1;
    const endX = Math.ceil((this.canvas.width - ox) / tile) + 1;
    const startY = Math.floor(-oy / tile) - 1;
    const endY = Math.ceil((this.canvas.height - oy) / tile) + 1;

    this.ctx.strokeStyle = "rgba(0, 0, 0, 0.18)";
    this.ctx.lineWidth = 1;
    this.ctx.beginPath();
    for (let x = startX; x <= endX; x++) {
      const px = Math.floor(ox + x * tile) + 0.5;
      this.ctx.moveTo(px, 0);
      this.ctx.lineTo(px, this.canvas.height);
    }
    for (let y = startY; y <= endY; y++) {
      const py = Math.floor(oy + y * tile) + 0.5;
      this.ctx.moveTo(0, py);
      this.ctx.lineTo(this.canvas.width, py);
    }
    this.ctx.stroke();
  }

  private drawActor(actor: Actor, tile: number, ox: number, oy: number): void {
    const x = ox + actor.x * tile;
    const y = oy + actor.y * tile;
    this.ctx.fillStyle = "#ffe8a0";
    this.ctx.strokeStyle = "#18120a";
    this.ctx.lineWidth = Math.max(2, tile * 0.15);
    this.ctx.beginPath();
    this.ctx.arc(x + tile / 2, y + tile / 2, tile * 0.45, 0, Math.PI * 2);
    this.ctx.fill();
    this.ctx.stroke();
  }
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

function superellipseDistance(dx: number, dy: number, power: number): number {
  return Math.pow(
    Math.pow(Math.abs(dx), power) + Math.pow(Math.abs(dy), power),
    1 / power,
  );
}

function smoothstep(value: number): number {
  const t = clamp(value, 0, 1);
  return t * t * (3 - 2 * t);
}

export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}
