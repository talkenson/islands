import type { RenderConfig } from "./types";

export const CHUNK_SIZE = 32;
export const TILE_SIZE = 12;
export const MIN_ZOOM = 0.35;
export const MAX_ZOOM = 4;

export const DEFAULT_RENDER_CONFIG: RenderConfig = {
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
