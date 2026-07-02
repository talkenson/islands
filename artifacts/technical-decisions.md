# Technical Decisions

## Core Stack

```text
Language: Go
Primary storage: PostgreSQL
Embedded prototype storage: bbolt, optional
World serialization: Protocol Buffers
Chunk payload compression: zstd
Chunk size: 32x32 cells
Map cell: one visible pixel = one gameplay tile
```

## Map Builder Module

Map generation is separated from the future game backend.

Current implementation:

```text
/internal/mapgen      deterministic map generation, binary save/load, PNG render
/cmd/mapbuilder       CLI tool that generates a saved world and preview image
```

The backend should consume saved chunk data. It should not own continent placement, terrain generation, noise settings, or preview rendering.

The first practical builder command is:

```text
go run ./cmd/mapbuilder -out artifacts/generated -seed talkenson -width 256 -height 256 -continents 3 -pixel-size 3
```

The builder currently ports the important visual/gameplay layers from `../canvas-map-gen`:

```text
continent placement
height/moisture/temperature noise
biome selection
biome palette based rendering
color noise variation
vegetation levels
dry bush
birch/pine/mixed forest
shallow coastal water
geology patches
rivers with meander and widening
```

Rivers are stored as `WaterRiver` cells. Their generated width is packed into `Water.Level`.

Dry bush is stored as `CoverDryBush`, separate from generic bush and forests.

It writes:

```text
artifacts/generated/world.islmap
artifacts/generated/world.png
```

`world.islmap` is a temporary compact binary format:

```text
magic:      ISLMAP01
header:     width, height, chunk size, chunk count
per chunk:  cx, cy, base[], water[], cover[], stock[], meta[]
```

This is intentionally not JSON. It saves the whole generated map as dense chunk arrays and already supports load round-trips. Later it can be replaced by the planned `zstd(protobuf(ChunkData))` codec without changing the generation/backend separation.

In the current builder, `Meta` stores generated height quantized to `0..255`. The generator keeps float height in memory while building the map so rivers, geology, and preview depth are not calculated from the coarse `Base.Elevation` field.

## Map Storage Model

The world is stored as:

```text
World -> Regions -> Chunks -> dense binary arrays + sparse entities
```

Do not store the map as `[]Cell` structs and do not store it as JSON.

Each chunk contains `32 * 32 = 1024` cells.

Dense per-cell state is stored in fixed-size arrays:

```go
const ChunkSize = 32
const ChunkCells = ChunkSize * ChunkSize

type Chunk struct {
    X int32
    Y int32

    Base  []uint16 // biome, soil, elevation, static flags
    Water []uint8  // water kind + water level/tide state
    Cover []uint16 // vegetation/road/field/etc + level
    Stock []uint16 // natural stock/resource amount
    Meta  []uint8  // fertility, moisture, pollution, visibility hints

    Dirty bool
}
```

`Base`, `Water`, `Cover`, `Stock`, and `Meta` must always have `ChunkCells` elements in memory.

## Cell Meaning

A cell is not a resource. A cell is a small piece of world.

Conceptual layers:

```text
building
vegetation / cover
water
soil
biome
```

Rendering priority:

```text
building > water > vegetation > soil > biome
```

Resource output is derived from tile state.

Examples:

```text
taiga + normal soil + pine forest level 4 + stock 78 = rich wood tile
river valley + fertile soil + field level 3 + farm building = food tile
coast + silt + tide water level 1 = reclaimable tidal land
```

## Suggested Bit Packing

Exact packing can change, but the first version should stay close to this.

### Base `uint16`

```text
bits 0..4    biome       0..31
bits 5..8    soil        0..15
bits 9..13   elevation   0..31
bits 14..15  flags       0..3
```

### Water `uint8`

```text
bits 0..3    water kind   0..15
bits 4..6    level        0..7
bit  7       tidal flag
```

### Cover `uint16`

```text
bits 0..7    cover kind   0..255
bits 8..11   level        0..15
bits 12..15  flags        0..15
```

### Stock `uint16`

`Stock` stores current natural stock for the main harvestable layer, such as wood in forest, food in field, reeds in marsh, or stone in rocky ground.

Use formulas and definitions to interpret stock, instead of storing many independent resources per cell.

## Entities Outside Chunks

Chunks only store dense terrain-like state.

Entities with ownership, timers, inventories, permissions, history, or behavior live outside chunks:

```text
actors
carts
ground piles
settlements
buildings
warehouses
projects
markets
events
visibility/discovery
```

Buildings should be entities, not just packed cell bits, once they need owner, HP, storage, production, or upgrades.

For rendering and collision, chunks may keep a lightweight occupancy/index layer later, but entity tables remain authoritative.

## Resource Location Rule

A harvested resource must always be in an explicit container.

Allowed physical states:

```text
1. In nature: tile stock.
2. In actor pocket: small items only.
3. In cart: bulk movable resources.
4. On ground: ground pile inventory.
5. In settlement/building: warehouse inventory.
```

There is no `MainlandStorage`.

Continents, islands, and regions are geography and politics. They do not magically hold resources.

## Inventory Model

Use one generic inventory system with several inventory kinds.

```go
type InventoryKind uint8

const (
    InventoryPocket InventoryKind = iota
    InventoryCart
    InventoryGroundPile
    InventorySettlementStorage
    InventoryBuildingStorage
)

type Inventory struct {
    ID       uint64
    WorldID  uint64
    Kind     InventoryKind

    OwnerType uint8
    OwnerID   uint64

    MaxWeight uint32
    MaxVolume uint32

    UpdatedTick uint64
}

type InventoryStack struct {
    InventoryID uint64
    ItemID      uint16
    Amount      uint32
    Quality     uint8
}
```

## Pocket And Cart Decision

Portable inventory is intentionally simple:

```text
Pocket = small personal inventory.
Cart = large physical movable container.
```

No backpack, hands, or separate body slots in MVP.

Pocket accepts:

```text
tools
keys
money
maps
seeds
small valuables
documents
```

Pocket rejects bulk resources:

```text
wood
stone
ore
large food stacks
building materials
```

Cart accepts bulk resources and can be:

```text
opened
closed
locked
broken
attached to actor
detached
stolen
dragged away
looted
```

Tools are auto-selected from pocket for actions.

Keys are items in pocket. Cart lock has a lock ID. Actor can open a locked cart if pocket contains a matching key.

## Cart Model

```go
type LockState uint8

const (
    LockOpen LockState = iota
    LockClosed
    LockBroken
)

type Cart struct {
    ID      uint64
    WorldID uint64

    X int32
    Y int32

    OwnerID uint64

    InventoryID uint64
    LockID      uint64
    LockState   LockState

    AttachedToActorID uint64
    HP                uint16
}
```

Ownership does not create magical protection.

It creates consequences:

```text
theft event
reputation loss
crime marker
retaliation rights
chat notification
```

## Harvest Flow

Forest harvest MVP flow:

```text
1. Actor stands on a forest cell.
2. Actor uses harvest action.
3. System auto-selects best axe/tool from pocket.
4. Tile stock decreases.
5. Bulk wood goes into attached/open accessible cart.
6. Overflow becomes ground pile.
7. Event is written.
8. Changed chunk is marked dirty.
```

If there is no cart, harvested bulk resources fall to a ground pile.

## Ground Pile

Ground piles are required even with carts.

They represent:

```text
overflow
dropped resources
loot
abandoned goods
destroyed storage remains
manual unloading
```

```go
type GroundPile struct {
    ID          uint64
    WorldID     uint64
    X           int32
    Y           int32
    InventoryID uint64
    CreatedBy   uint64
    CreatedTick uint64
    DecayTick   uint64
}
```

## World Growth

Chat size should not instantly resize a continent.

Growth becomes gameplay:

```text
migration events
tide recession
new sandbars
reclaimable land
nearby islands
frontier camps
colonial expeditions
```

Use effective activity instead of raw member count.

Suggested formula direction:

```text
effective_size = smoothed(active_players_7d)
growth_factor = sqrt(effective_size)
```

Expansion requires projects and resources.

New land should often start as weak or bare:

```text
water
tide
silt
sand
bare earth
poor soil
marsh
rocky ground
```

Players improve it through:

```text
dams
drainage
planting
roads
bridges
ports
warehouses
settlements
```

If activity falls, land does not disappear. Instead, remote regions become inefficient, abandoned, or reclaimed by nature.

## Persistence

PostgreSQL tables should separate chunk blobs from game entities.

Chunk table:

```sql
CREATE TABLE world_chunks (
    world_id BIGINT NOT NULL,
    cx INT NOT NULL,
    cy INT NOT NULL,
    version INT NOT NULL,
    updated_tick BIGINT NOT NULL,
    data BYTEA NOT NULL,
    PRIMARY KEY (world_id, cx, cy)
);
```

`data` is:

```text
zstd(protobuf(ChunkData))
```

Event log:

```sql
CREATE TABLE world_events (
    id BIGSERIAL PRIMARY KEY,
    world_id BIGINT NOT NULL,
    tick BIGINT NOT NULL,
    actor_id BIGINT,
    event_type SMALLINT NOT NULL,
    payload BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Inventory tables:

```sql
CREATE TABLE inventories (
    id BIGSERIAL PRIMARY KEY,
    world_id BIGINT NOT NULL,
    kind SMALLINT NOT NULL,
    owner_type SMALLINT NOT NULL,
    owner_id BIGINT,
    max_weight INT NOT NULL,
    max_volume INT NOT NULL,
    updated_tick BIGINT NOT NULL
);

CREATE TABLE inventory_stacks (
    inventory_id BIGINT NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    item_id INT NOT NULL,
    quality SMALLINT NOT NULL DEFAULT 0,
    amount BIGINT NOT NULL,
    PRIMARY KEY (inventory_id, item_id, quality)
);
```

## Protobuf Chunk Shape

```proto
syntax = "proto3";

package islands.world.v1;

message ChunkData {
  int32 x = 1;
  int32 y = 2;

  uint32 version = 3;
  uint64 updated_tick = 4;

  bytes base = 10;   // 1024 * uint16 little-endian
  bytes water = 11;  // 1024 * uint8
  bytes cover = 12;  // 1024 * uint16 little-endian
  bytes stock = 13;  // 1024 * uint16 little-endian
  bytes meta = 14;   // 1024 * uint8
}
```

Avoid `repeated Cell`.

## Size Estimate

Per cell:

```text
Base  2 bytes
Water 1 byte
Cover 2 bytes
Stock 2 bytes
Meta  1 byte
Total 8 bytes/cell raw
```

Per `32x32` chunk:

```text
1024 cells * 8 bytes = 8192 bytes raw
```

Map examples:

```text
256x256   = ~512 KB raw
512x512   = ~2 MB raw
1024x1024 = ~8 MB raw
2048x2048 = ~32 MB raw
```

zstd should compress natural terrain chunks well, often substantially below raw size.
