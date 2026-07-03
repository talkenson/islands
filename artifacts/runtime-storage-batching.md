# Runtime Storage Batching And Compaction

## Current Runtime Model

The generated world map is loaded into memory at server startup.

Startup order when `-world-map` is provided:

```text
1. Read base `world.islmap`.
2. Apply `world.journal` records over the base chunks.
3. Read `world.players.json`.
4. Keep the active world state in `game.Service` memory.
```

During normal gameplay, the base `world.islmap` is not rewritten for every action.

Runtime writes are:

```text
world.journal        append-only dirty chunk snapshots
world.players.json   latest actor/inventory state
```

Map reads from disk happen only on startup or explicit compact/load flows.

## Batching Decision

Runtime storage uses a write-behind batching wrapper over `FileStore`.

Dirty chunk writes are coalesced by chunk coordinate within a batch:

```text
same chunk changed many times -> write only latest chunk snapshot
```

Player state writes are also coalesced:

```text
many moves/inventory changes -> write only latest player state
```

Default runtime flags:

```text
-storage-batch-interval 1s
-storage-batch-max-chunks 128
```

Synchronous mode is still available:

```text
-storage-batch-interval 0
```

Tradeoff: on process crash, the server can lose up to the latest batch interval of persisted changes. On graceful shutdown, pending storage is flushed.

## Journal Writing

`FileStore` supports batch dirty chunk writes with one journal open and one `Sync()` per batch.

This reduces disk pressure compared with the previous behavior:

```text
old: each dirty chunk -> open journal -> write record -> fsync
new: dirty chunk batch -> open journal -> write records -> fsync
```

The journal still stores full chunk snapshots, not deltas. This keeps replay simple and robust.

## Compaction

Compaction rewrites the compacted in-memory world back into `world.islmap` and clears `world.journal`.

Manual one-shot compaction:

```bash
go run ./cmd/islands \
  -world-map artifacts/generated/world.islmap \
  -compact-world
```

Optional startup compaction:

```text
-compact-world-on-start
```

Optional periodic runtime compaction:

```text
-compact-world-interval 15m
```

Periodic compaction is disabled by default. When enabled, compaction goes through `game.Service.CompactWorld`, which holds the service lock while snapshotting/compacting. Actions wait during compaction, but this avoids the race:

```text
snapshot world -> action writes journal -> compact clears journal
```

## Future World Simulation Note

Future background simulation should mutate only chunks that are currently active around loaded players, for example each player's visible `3x3` chunk window.

Those simulation updates should use the same dirty chunk path as player actions:

```text
simulation mutates in-memory chunk
mark chunk dirty
batching store coalesces and persists latest chunk snapshot
realtime publishes relevant chunk/entity updates
```

This keeps simulation, player actions, journal persistence, and compaction on one storage contract.
