# Islands

Backend and map-builder prototype for a chat-based territorial strategy game.

## Generate A Map

Run the standalone map builder:

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/mapbuilder
```

By default it writes a preview render to:

```text
artifacts/generated/world.png
```

`world.png` is a preview render.  
Pass `-export-map` to also write `world.islmap`, the saved binary map data.

## Useful Flags

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/mapbuilder \
  -out artifacts/generated \
  -seed talkenson \
  -width 256 \
  -height 256 \
  -workers 4 \
  -continents 3 \
  -rivers 12 \
  -min-river-length 20 \
  -pixel-size 3 \
  -export-map \
  -timings
```

Flags:

- `-out`: output directory.
- `-seed`: deterministic generation seed.
- `-width`: map width in cells.
- `-height`: map height in cells.
- `-workers`: parallel map generation workers; `0` uses up to 8 logical CPUs.
- `-continents`: target continent/island count.
- `-rivers`: target river count.
- `-min-river-length`: minimum accepted river length.
- `-pixel-size`: preview PNG scale, in rendered pixels per map cell.
- `-export-map`: write generated binary map data to `world.islmap`.
- `-timings`: print map generation stage timings.

## Output

Example output:

```text
generated 256x256 world with 64 chunks
land=12074 water=53462 shallow=1813 river=919 forest=1431 dry_bush=163 rock=677 mountain=0 wood=16167 stone=4400
saved render: artifacts/generated/world.png
```

## Run The Backend

Generate an exported map first:

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/mapbuilder \
  -out artifacts/generated \
  -width 256 \
  -height 256 \
  -workers 4 \
  -export-map
```

Then start the API server with that map:

```bash
GOCACHE=/home/talk/work/islands/.gocache go run ./cmd/islands \
  -world-map artifacts/generated/world.islmap \
  -visible-chunk-radius 1
```

Use `-visible-chunk-radius 2` for a `5x5` live chunk window. If `-world-map` is omitted, the server starts with a small in-memory demo world.
`world.islmap` stores the generator seed used by the frontend render palette/noise.

## Frontend

The client is a Solid + Vite app in `client/`.

For development, start the Go backend on `:8080`, then run:

```bash
cd client
npm install
npm run dev
```

For the Go server to serve the built frontend directly:

```bash
cd client
npm run build
```

The backend serves `client/dist`; use Vite while developing unbuilt TSX sources.

## Tests

Run tests with a local Go build cache:

```bash
GOCACHE=/home/talk/work/islands/.gocache go test ./...
```

The explicit `GOCACHE` is useful in this workspace because the default home cache may be read-only.
