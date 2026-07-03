import { CHUNK_SIZE } from "./config";
import { chunkKey } from "./chunks";
import type { ChunkSnapshot, WorldCell } from "./types";

const BIOMES: Record<number, string> = {
  0: "unknown",
  1: "taiga",
  2: "birch forest",
  3: "temperate forest",
  4: "river valley",
  5: "coast",
  6: "marsh",
  7: "steppe",
  8: "mountain",
  9: "desert",
  10: "meadow",
};

const SOILS: Record<number, string> = {
  0: "unknown",
  1: "water",
  2: "silt",
  3: "sand",
  4: "bare",
  5: "grass",
  6: "fertile",
  7: "exhausted",
  8: "rocky",
  9: "marsh",
};

const WATER: Record<number, string> = {
  0: "none",
  1: "sea",
  2: "river",
  3: "lake",
  4: "canal",
  5: "swamp",
};

const COVER: Record<number, string> = {
  0: "none",
  1: "grass",
  2: "bush",
  3: "dry bush",
  4: "birch forest",
  5: "pine forest",
  6: "mixed forest",
  7: "reeds",
  8: "field",
  9: "road",
};

export interface CellMeta {
  cell: WorldCell;
  cx: number;
  cy: number;
  index: number;
  biome: string;
  soil: string;
  elevation: number;
  baseFlags: number;
  water: string;
  waterLevel: number;
  waterTidal: boolean;
  cover: string;
  coverLevel: number;
  coverFlags: number;
  stock: number;
  height: number;
  temperature: number;
  updatedTick: number;
}

export function readCellMeta(
  chunks: Map<string, ChunkSnapshot>,
  cell: WorldCell,
): CellMeta | undefined {
  const cx = Math.floor(cell.x / CHUNK_SIZE);
  const cy = Math.floor(cell.y / CHUNK_SIZE);
  const lx = ((cell.x % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
  const ly = ((cell.y % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
  const index = ly * CHUNK_SIZE + lx;
  const chunk = chunks.get(chunkKey(cx, cy));
  if (!chunk) {
    return undefined;
  }

  const base = chunk.base[index] || 0;
  const water = chunk.water[index] || 0;
  const cover = chunk.cover[index] || 0;
  const biome = base & 31;
  const soil = (base >> 5) & 15;
  const elevation = (base >> 9) & 31;
  const waterKind = water & 15;
  const coverKind = cover & 255;

  return {
    cell,
    cx,
    cy,
    index,
    biome: BIOMES[biome] || `biome ${biome}`,
    soil: SOILS[soil] || `soil ${soil}`,
    elevation,
    baseFlags: (base >> 14) & 3,
    water: WATER[waterKind] || `water ${waterKind}`,
    waterLevel: (water >> 4) & 7,
    waterTidal: (water & 128) !== 0,
    cover: COVER[coverKind] || `cover ${coverKind}`,
    coverLevel: (cover >> 8) & 15,
    coverFlags: (cover >> 12) & 15,
    stock: chunk.stock[index] || 0,
    height: chunk.meta.length > index ? chunk.meta[index] || 0 : elevation,
    temperature:
      chunk.temperature.length > index ? chunk.temperature[index] || 0 : 0,
    updatedTick: chunk.updatedTick,
  };
}
