# Implementation Plan

## Product Direction

We are building a social territorial strategy for chat communities.

The core experience is not a full RimWorld-like simulation. The MVP should be a manageable backend game where players act through commands, shape a visible pixel map, move resources physically, and create social stories through ownership, logistics, trade, theft, voting, and local conflict.

Main principle:

```text
Simple simulation rules, strong social consequences.
```

## Fixed MVP Scope

### Included

- One shared world map.
- Each chat starts on its own island, continent, or archipelago.
- World is stored as chunks.
- Each map cell is one visible pixel and one gameplay tile.
- Tile has layered state: biome, soil, water, vegetation, building, stock/level.
- Fog/visibility hides undiscovered land.
- Chat growth opens new expansion opportunities through tides, islands, migration, and exploration.
- Natural resources are local, not global.
- Harvested resources become physical inventory stacks.
- Player has a small pocket for tools, keys, money, maps, and small valuables.
- Cart is a movable physical container for bulk resources.
- Ground piles exist for overflow, drops, theft, and abandoned resources.
- Settlement warehouses store communal resources.
- Basic actions: move, harvest, plant, build, deposit, withdraw, attach cart, open cart, close cart.
- Event log records important changes.

### Explicitly Deferred

- Full NPC pathfinding.
- Individual worker simulation.
- Complex combat.
- Electricity.
- Large tech tree.
- Diseases and detailed citizen needs.
- Factorio-style production chains.
- Formal secret clan system.

## Milestones

### Milestone 1: Backend Skeleton

Goal: create a Go service with stable domain packages and tests for core structures.

Deliverables:

- Go module.
- Package layout under `internal`.
- Basic config.
- Domain identifiers and enums.
- Unit tests for cell packing and coordinates.

Suggested packages:

```text
/cmd/islands
/internal/world
/internal/actor
/internal/inventory
/internal/storage
/internal/codec
/internal/sim
/internal/game
/internal/eventlog
/api/proto
```

### Milestone 2: World And Chunk Model

Goal: represent the map efficiently in memory and on disk.

Deliverables:

- `ChunkSize = 32`.
- `Chunk` with dense arrays for base, water, cover, stock/meta.
- Cell packing helpers.
- Coordinate conversion: world position to chunk coord and local index.
- Dirty chunk tracking.
- Simple world generator for one island.

### Milestone 3: Codec And Storage

Goal: persist chunks without JSON.

Deliverables:

- Protobuf schema for chunk payloads.
- Dense binary arrays inside protobuf `bytes`.
- zstd compression around protobuf payload.
- Storage interface.
- Initial storage implementation.

Decision:

For serious backend work prefer PostgreSQL. For a very fast local prototype bbolt is acceptable, but the main design should target PostgreSQL.

### Milestone 4: Inventory And Physical Resources

Goal: make resources local and movable.

Deliverables:

- Generic inventory container model.
- Inventory stack table/model.
- Pocket inventory for each actor.
- Cart entity with inventory, lock state, owner, position, attached actor.
- Ground pile entity with inventory and decay tick.
- Settlement warehouse inventory.
- Item/resource definitions with tags, weight, volume, stack size.

### Milestone 5: First Game Actions

Goal: create a playable backend loop.

Deliverables:

- Create actor.
- Move actor.
- Attach/detach cart.
- Open/close cart with key.
- Harvest forest with tool auto-selection from pocket.
- Put bulk harvest into cart.
- Overflow to ground pile.
- Deposit cart resources into settlement warehouse.
- Emit events for all meaningful actions.

### Milestone 6: Natural Simulation

Goal: make the map feel alive.

Deliverables:

- Forest growth tick.
- Soil fertility/depletion stages.
- Tide cells and reclaimed land.
- Planting forest.
- Basic farm/food production.
- Dirty chunk flushing after sim ticks.

### Milestone 7: Chat Growth And Expansion

Goal: turn chat growth into gameplay instead of free land.

Deliverables:

- Effective activity metric.
- Smoothed chat size calculation.
- Expansion thresholds.
- Migration events.
- Tide/reclaimed land opportunities.
- Exploration projects for nearby islands.

### Milestone 8: Bot/API Layer

Goal: expose the game to Telegram or another chat interface.

Initial commands:

```text
/map
/me
/move
/cart
/harvest
/plant
/build
/warehouse
/projects
/events
```

The backend should not depend directly on Telegram. Bot handlers should call application services in `internal/game`.

## Development Priorities

1. Data structures must stay compact and testable.
2. The map is authoritative, but complex entities live outside chunks.
3. No JSON for world state.
4. No per-cell Go structs for the dense map.
5. Resource location must always be explicit.
6. Ownership should create consequences, not magical restrictions.
7. Use event log for debugging, replay, audit, and season summaries.

