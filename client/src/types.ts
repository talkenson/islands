export type StatusKind = "idle" | "live" | "error";
export type ActionType = "move" | "harvest";

export interface Actor {
  id: number;
  world_id: number;
  x: number;
  y: number;
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
}

export interface LoginResponse {
  token: string;
  user_id: number;
  actors: ActorWire[];
  worlds: Array<{ id: number }>;
}

export interface ChunkSnapshotWire {
  cx: number;
  cy: number;
  base?: number[];
  water?: number[] | string;
  cover?: number[];
  stock?: number[];
  meta?: number[] | string;
  updated_tick?: number;
}

export interface ChunkSnapshot {
  cx: number;
  cy: number;
  base: number[];
  water: Uint8Array;
  cover: number[];
  stock: number[];
  meta: Uint8Array;
  updatedTick: number;
}

export interface ActionResult {
  accepted?: boolean;
  client_action_id?: string;
  actor?: ActorWire;
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
  render_config?: RenderConfig;
}

export interface EntityPatchPayload extends ActionResult {
  actor?: ActorWire;
}

export interface ChunkPosition {
  cx: number;
  cy: number;
  index: number;
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
