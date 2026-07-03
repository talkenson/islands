import { CHUNK_SIZE } from "./config";
import type {
  Actor,
  ActorWire,
  ChunkPosition,
  ChunkSnapshot,
  ChunkSnapshotWire,
  InventoryItem,
  InventoryItemWire,
} from "./types";

export function chunkKey(cx: number, cy: number): string {
  return `${cx},${cy}`;
}

export function normalizeActor(actor: ActorWire, fallbackWorldID: number): Actor {
  return {
    id: actor.ID || actor.id || 1,
    world_id: actor.WorldID || actor.world_id || fallbackWorldID,
    x: actor.X ?? actor.x ?? 0,
    y: actor.Y ?? actor.y ?? 0,
    inventory_id:
      actor.InventoryID ??
      actor.inventory_id ??
      actor.PocketInventoryID ??
      actor.pocket_inventory_id ??
      0,
  };
}

export function normalizeInventory(items: InventoryItemWire[] | undefined): InventoryItem[] {
  return (items || [])
    .map((item) => ({
      item_id: item.ItemID ?? item.item_id ?? 0,
      name: item.Name || item.name || "unknown",
      amount: item.Amount ?? item.amount ?? 0,
      quality: item.Quality ?? item.quality ?? 0,
    }))
    .filter((item) => item.item_id > 0 && item.amount > 0);
}

export function normalizeChunk(data: ChunkSnapshotWire): ChunkSnapshot {
  return {
    cx: data.cx,
    cy: data.cy,
    base: decodeUint16Layer(data.base),
    water: decodeByteLayer(data.water),
    cover: decodeUint16Layer(data.cover),
    stock: decodeUint16Layer(data.stock),
    meta: decodeByteLayer(data.meta),
    temperature: decodeByteLayer(data.temperature),
    updatedTick: data.updated_tick || 0,
  };
}

export function actorChunkAndIndex(actor: Actor): ChunkPosition {
  const cx = Math.floor(actor.x / CHUNK_SIZE);
  const cy = Math.floor(actor.y / CHUNK_SIZE);
  const lx = ((actor.x % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
  const ly = ((actor.y % CHUNK_SIZE) + CHUNK_SIZE) % CHUNK_SIZE;
  return { cx, cy, index: ly * CHUNK_SIZE + lx };
}

function decodeByteLayer(value: number[] | string | undefined): Uint8Array {
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

function decodeUint16Layer(value: number[] | string | undefined): Uint16Array {
  if (!value) {
    return new Uint16Array();
  }
  if (Array.isArray(value)) {
    return Uint16Array.from(value);
  }

  const binary = atob(value);
  const out = new Uint16Array(Math.floor(binary.length / 2));
  for (let i = 0; i < out.length; i++) {
    out[i] = binary.charCodeAt(i * 2) | (binary.charCodeAt(i * 2 + 1) << 8);
  }
  return out;
}
