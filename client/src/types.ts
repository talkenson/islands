export type StatusKind = "idle" | "live" | "error";
export type ActionType = "move" | "harvest";
export type PanelID = "actor" | "cell" | "inventory" | "controls" | "events";

export interface LogEvent {
  id: number;
  title: string;
  detail?: string | number;
}

export interface Actor {
  id: number;
  world_id: number;
  x: number;
  y: number;
  inventory_id: number;
}

export interface InventoryItem {
  item_id: number;
  name: string;
  amount: number;
  quality?: number;
}

export interface InventoryItemWire {
  ItemID?: number;
  item_id?: number;
  Name?: string;
  name?: string;
  Amount?: number;
  amount?: number;
  Quality?: number;
  quality?: number;
}

export interface ActorWire {
  ID?: number;
  id?: number;
  WorldID?: number;
  world_id?: number;
  X?: number;
  x?: number;
  Y?: number;
  y?: number;
  PocketInventoryID?: number;
  pocket_inventory_id?: number;
  InventoryID?: number;
  inventory_id?: number;
}

export interface LoginResponse {
  token: string;
  user_id: number;
  actors: ActorWire[];
  inventory?: InventoryItemWire[];
  worlds: Array<{ id: number }>;
}

export interface ChunkSnapshotWire {
  cx: number;
  cy: number;
  base?: number[] | string;
  water?: number[] | string;
  cover?: number[] | string;
  stock?: number[] | string;
  meta?: number[] | string;
  temperature?: number[] | string;
  updated_tick?: number;
}

export interface ChunkSnapshot {
  cx: number;
  cy: number;
  base: Uint16Array;
  water: Uint8Array;
  cover: Uint16Array;
  stock: Uint16Array;
  meta: Uint8Array;
  temperature: Uint8Array;
  updatedTick: number;
}

export interface ActionResult {
  accepted?: boolean;
  client_action_id?: string;
  action_type?: ActionType;
  event_id?: number;
  code?: string;
  message?: string;
}

export interface RealtimeEvent {
  id?: number;
  type?: string;
  data?: unknown;
}

export interface HelloPayload {
  actor_id: number;
  world_id: number;
  actor?: ActorWire;
  inventory?: InventoryItemWire[];
  render_config?: RenderConfig;
  world_time?: WorldTime;
}

export interface EntityPatchPayload {
  actor?: ActorWire;
}

export interface InventoryPatchPayload {
  actor_id: number;
  inventory_id: number;
  inventory?: InventoryItemWire[];
}

export interface ChunkPosition {
  cx: number;
  cy: number;
  index: number;
}

export interface WorldCell {
  x: number;
  y: number;
}

export type DayPhase =
  | "late_night"
  | "dawn"
  | "morning"
  | "day"
  | "afternoon"
  | "dusk"
  | "evening"
  | "night";

export interface WorldTime {
  world_time: number;
  day: number;
  phase: DayPhase;
  phase_progress: number;
  day_progress: number;
  day_length_seconds: number;
  world_seconds_per_real_second: number;
}

export interface RenderConfig {
  seed: string;
  palette: RenderPalette;
}

export interface RenderPalette {
  water: {
    sea: string;
    lake: string;
    tidal_sea: string;
    tidal_lake: string;
    river: string;
    river_variant: string;
  };
  terrain: {
    rock: string;
    coastal_rock: string;
    sand: string;
    mountain_light: string;
    mountain_dark: string;
  };
  cover: {
    birch_forest: string;
    pine_forest: string;
    mixed_forest: string;
    dry_bush: string;
    bush: string;
  };
  biomes: Record<string, string[]>;
}

export interface Viewport {
  scale: number;
  ox: number;
  oy: number;
  zoom: number;
}
