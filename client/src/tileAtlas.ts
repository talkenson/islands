export const TEXTURED_TILE_PIXELS = 16;
const MAX_STAMP_CACHE_SIZE = 4096;

export const MASK_N = 1;
export const MASK_E = 2;
export const MASK_S = 4;
export const MASK_W = 8;

export type TileKind =
  | "grass"
  | "sand"
  | "rock"
  | "mountain"
  | "water"
  | "river"
  | "forest"
  | "bush";

export type SurfaceStyle = "trail" | "dirt_road" | "stone_road" | "fence";

export interface SurfacePaint {
  color: string;
  style: SurfaceStyle;
  mask: number;
  variant: number;
}

export interface TilePaint {
  baseColor: string;
  kind: TileKind;
  variant: number;
  density?: number;
  surface?: SurfacePaint;
}

interface AtlasImage {
  canvas: HTMLCanvasElement;
  context: CanvasRenderingContext2D;
}

interface AtlasManifest {
  tileSize: number;
  palette?: Array<{
    id: string;
    label: string;
    type: "transparent" | "shade" | "color";
    rgba: [number, number, number, number];
  }>;
  atlases: {
    terrain: {
      file: string;
      slots: Array<{ kind: TileKind; index: number }>;
    };
    surface: {
      file: string;
      rowsByStyle: Record<SurfaceStyle, number>;
    };
  };
}

const manifestURL = "/assets/tiles/atlas-manifest.json";

const TERRAIN_SLOTS: Record<TileKind, readonly number[]> = {
  grass: [0, 1],
  sand: [2, 3],
  rock: [4, 5],
  mountain: [6, 7],
  water: [8, 9],
  river: [10, 11],
  forest: [12, 13],
  bush: [14, 15],
};

const SURFACE_ROWS: Record<SurfaceStyle, number> = {
  trail: 0,
  dirt_road: 1,
  stone_road: 2,
  fence: 3,
};

export class TileAtlas {
  private readonly stampCache = new Map<string, HTMLCanvasElement>();
  private terrainAtlas: AtlasImage | undefined;
  private surfaceAtlas: AtlasImage | undefined;
  private terrainSlots: Record<TileKind, readonly number[]> = TERRAIN_SLOTS;
  private surfaceRows: Record<SurfaceStyle, number> = SURFACE_ROWS;
  private readonly readyCallbacks: Array<() => void> = [];
  private loaded = false;

  constructor() {
    void this.load();
  }

  drawTile(
    context: CanvasRenderingContext2D,
    paint: TilePaint,
    dx: number,
    dy: number,
  ): void {
    context.fillStyle = paint.baseColor;
    context.fillRect(dx, dy, TEXTURED_TILE_PIXELS, TEXTURED_TILE_PIXELS);

    const terrainStamp = this.terrainStampFor(paint);
    if (terrainStamp) {
      context.drawImage(terrainStamp, dx, dy);
    }

    if (paint.surface) {
      const surfaceStamp = this.surfaceStampFor(paint.surface);
      if (surfaceStamp) {
        context.drawImage(surfaceStamp, dx, dy);
      }
    }
  }

  clear(): void {
    this.stampCache.clear();
  }

  whenReady(callback: () => void): void {
    if (this.loaded) {
      callback();
      return;
    }
    this.readyCallbacks.push(callback);
  }

  private async load(): Promise<void> {
    const manifest = await loadManifest(manifestURL);
    this.terrainSlots = terrainSlotsFromManifest(manifest);
    this.surfaceRows = manifest.atlases.surface.rowsByStyle;
    const [terrainAtlas, surfaceAtlas] = await Promise.all([
      loadAtlasImage(atlasURL(manifest.atlases.terrain.file)),
      loadAtlasImage(atlasURL(manifest.atlases.surface.file)),
    ]);
    this.terrainAtlas = terrainAtlas;
    this.surfaceAtlas = surfaceAtlas;
    this.loaded = true;
    this.readyCallbacks.splice(0).forEach((callback) => callback());
  }

  private terrainStampFor(paint: TilePaint): HTMLCanvasElement | undefined {
    if (!this.terrainAtlas) {
      return undefined;
    }

    const slots = this.terrainSlots[paint.kind];
    const densityShift = Math.round((paint.density || 0) * 3);
    const slot = slots[(paint.variant + densityShift) % slots.length];
    const key = `terrain:${slot}:${normalizeColorKey(paint.baseColor)}`;
    return this.cachedStamp(key, this.terrainAtlas, slot, paint.baseColor);
  }

  private surfaceStampFor(
    surface: SurfacePaint,
  ): HTMLCanvasElement | undefined {
    if (!this.surfaceAtlas) {
      return undefined;
    }

    const row = this.surfaceRows[surface.style];
    const slot = row * 16 + (surface.mask & 15);
    const key = `surface:${slot}:${normalizeColorKey(surface.color)}`;
    return this.cachedStamp(key, this.surfaceAtlas, slot, surface.color);
  }

  private cachedStamp(
    key: string,
    atlas: AtlasImage,
    slot: number,
    color: string,
  ): HTMLCanvasElement {
    const cached = this.stampCache.get(key);
    if (cached) {
      return cached;
    }

    const stamp = tintAtlasSlot(atlas, slot, color);
    if (this.stampCache.size >= MAX_STAMP_CACHE_SIZE) {
      this.stampCache.clear();
    }
    this.stampCache.set(key, stamp);
    return stamp;
  }
}

async function loadManifest(url: string): Promise<AtlasManifest> {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`failed to load atlas manifest ${url}`);
  }
  return (await response.json()) as AtlasManifest;
}

function terrainSlotsFromManifest(
  manifest: AtlasManifest,
): Record<TileKind, readonly number[]> {
  const slots: Record<TileKind, number[]> = {
    grass: [],
    sand: [],
    rock: [],
    mountain: [],
    water: [],
    river: [],
    forest: [],
    bush: [],
  };
  for (const slot of manifest.atlases.terrain.slots) {
    slots[slot.kind].push(slot.index);
  }
  return slots;
}

function atlasURL(file: string): string {
  return `/assets/tiles/${file}`;
}

function loadAtlasImage(url: string): Promise<AtlasImage> {
  return new Promise((resolve, reject) => {
    const image = new Image();
    image.onload = () => {
      const canvas = document.createElement("canvas");
      canvas.width = image.naturalWidth;
      canvas.height = image.naturalHeight;
      const context = canvas.getContext("2d", { willReadFrequently: true });
      if (!context) {
        reject(new Error("2d atlas context is unavailable"));
        return;
      }
      context.imageSmoothingEnabled = false;
      context.drawImage(image, 0, 0);
      resolve({ canvas, context });
    };
    image.onerror = () => reject(new Error(`failed to load atlas ${url}`));
    image.src = url;
  });
}

function tintAtlasSlot(
  atlas: AtlasImage,
  slot: number,
  color: string,
): HTMLCanvasElement {
  const columns = Math.max(
    1,
    Math.floor(atlas.canvas.width / TEXTURED_TILE_PIXELS),
  );
  const sx = (slot % columns) * TEXTURED_TILE_PIXELS;
  const sy = Math.floor(slot / columns) * TEXTURED_TILE_PIXELS;
  const source = atlas.context.getImageData(
    sx,
    sy,
    TEXTURED_TILE_PIXELS,
    TEXTURED_TILE_PIXELS,
  );
  const tinted = new ImageData(TEXTURED_TILE_PIXELS, TEXTURED_TILE_PIXELS);
  const base = parseColor(color);

  for (let i = 0; i < source.data.length; i += 4) {
    const alpha = source.data[i + 3];
    if (alpha === 0) {
      continue;
    }

    const r = source.data[i];
    const g = source.data[i + 1];
    const b = source.data[i + 2];
    if (isGrayscale(r, g, b)) {
      const shade = r - 128;
      tinted.data[i] = clampChannel(base[0] + shade);
      tinted.data[i + 1] = clampChannel(base[1] + shade);
      tinted.data[i + 2] = clampChannel(base[2] + shade);
    } else {
      tinted.data[i] = r;
      tinted.data[i + 1] = g;
      tinted.data[i + 2] = b;
    }
    tinted.data[i + 3] = alpha;
  }

  const canvas = document.createElement("canvas");
  canvas.width = TEXTURED_TILE_PIXELS;
  canvas.height = TEXTURED_TILE_PIXELS;
  const context = canvas.getContext("2d");
  if (!context) {
    throw new Error("2d tinted stamp context is unavailable");
  }
  context.imageSmoothingEnabled = false;
  context.putImageData(tinted, 0, 0);
  return canvas;
}

function normalizeColorKey(color: string): string {
  return color.replace(/\s+/g, "").toLowerCase();
}

function isGrayscale(r: number, g: number, b: number): boolean {
  return r === g && g === b;
}

function parseColor(color: string): [number, number, number] {
  if (color.startsWith("#")) {
    const hex = color.slice(1);
    return [
      parseInt(hex.slice(0, 2), 16),
      parseInt(hex.slice(2, 4), 16),
      parseInt(hex.slice(4, 6), 16),
    ];
  }

  const match = color.match(/rgb\((\d+),\s*(\d+),\s*(\d+)\)/);
  if (!match) {
    return [128, 128, 128];
  }
  return [
    Number.parseInt(match[1], 10),
    Number.parseInt(match[2], 10),
    Number.parseInt(match[3], 10),
  ];
}

function clampChannel(value: number): number {
  return Math.max(0, Math.min(255, Math.round(value)));
}
