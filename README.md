# Islands

Backend and map-builder prototype for a chat-based territorial strategy game.

## Generate A Map

Run the standalone map builder:

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/mapbuilder
```

By default it writes generated files to:

```text
artifacts/generated/world.islmap
artifacts/generated/world.png
```

`world.islmap` is the saved binary map data.  
`world.png` is a preview render.

## Useful Flags

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/mapbuilder \
  -out artifacts/generated \
  -seed talkenson \
  -width 256 \
  -height 256 \
  -continents 3 \
  -rivers 12 \
  -min-river-length 20 \
  -pixel-size 3
```

Flags:

- `-out`: output directory.
- `-seed`: deterministic generation seed.
- `-width`: map width in cells.
- `-height`: map height in cells.
- `-continents`: target continent/island count.
- `-rivers`: target river count.
- `-min-river-length`: minimum accepted river length.
- `-pixel-size`: preview PNG scale, in rendered pixels per map cell.

## Output

Example output:

```text
generated 256x256 world with 64 chunks
land=12074 water=53462 shallow=1813 river=919 forest=1431 dry_bush=163 rock=677 mountain=0 wood=16167 stone=4400
saved map: artifacts/generated/world.islmap
saved render: artifacts/generated/world.png
```

## Tests

Run tests with a local Go build cache:

```bash
GOCACHE=/home/talk/work/islands/.gocache go test ./...
```

The explicit `GOCACHE` is useful in this workspace because the default home cache may be read-only.

