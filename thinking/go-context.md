давай подумаем, я хочу сделать бэк и написать всю карту (а в json ее хранить категорически нельзя) на golang. давай подумаем над структурами данных, сколько это займет места, какие технологии и сжатие (gzip/pb/...) использовать? 

Да, JSON тут лучше вообще не трогать для состояния карты. Я бы делал так:

**идеальная базовая архитектура:**
`World -> Chunks -> dense binary arrays + sparse objects`

То есть карта не как массив `Cell`, а как чанки по `32x32` или `64x64`, где свойства клеток лежат отдельными плотными массивами байтов/uint16, а здания/юниты/редкие объекты — отдельными списками.

## 1. Главная идея: клетка не должна быть Go-struct’ом

Плохой вариант:

```go
type Cell struct {
    Biome      uint8
    Soil       uint8
    Cover      uint8
    Level      uint8
    Resource   uint16
    BuildingID uint32
    Flags      uint16
}
```

Даже если кажется, что это “всего 12 байт”, из-за выравнивания, мусора в полях и будущих расширений оно быстро раздуется. И главное — для сериализации, диффов и сжатия это хуже.

Лучше так:

```go
const ChunkSize = 32
const ChunkCells = ChunkSize * ChunkSize

type Chunk struct {
    X int32
    Y int32

    Base  []uint16 // biome, soil, height, flags
    Cover []uint16 // forest/field/water/road/etc + level + resource
    Meta  []uint8  // fertility/moisture/temperature/etc, если надо

    Buildings []BuildingRef
    Dirty     bool
}
```

Где `Base`, `Cover`, `Meta` всегда длиной `1024` для чанка `32x32`.

## 2. Пример упаковки клетки

### Base: `uint16`

```text
bits 0..4   biome      0..31
bits 5..8   soil       0..15
bits 9..13  height     0..31
bits 14..15 flags      0..3
```

```go
type BaseCell uint16

func PackBase(biome, soil, height, flags uint16) BaseCell {
    return BaseCell(
        (biome & 31) |
            ((soil & 15) << 5) |
            ((height & 31) << 9) |
            ((flags & 3) << 14),
    )
}
```

### Cover: `uint16`

```text
bits 0..5    cover type      0..63
bits 6..9    level           0..15
bits 10..15  resource amount 0..63
```

Например:

```go
type CoverType uint8

const (
    CoverNone CoverType = iota
    CoverGrass
    CoverBush
    CoverForestBirch
    CoverForestPine
    CoverField
    CoverWater
    CoverRoad
    CoverRuins
)
```

`resource amount` лучше хранить не как “точное дерево = 173 единицы”, а как уровень `0..63`, а реальное число считать формулой:

```go
wood := baseWoodByCover[coverType] * resourceLevel
```

Так карта будет сильно компактнее.

## 3. Здания — не в каждой клетке

Не надо класть `BuildingID` в каждую клетку. Большинство клеток будет без зданий.

Лучше:

```go
type BuildingRef struct {
    CellIndex uint16 // 0..1023 внутри чанка
    Type      uint16
    OwnerID   uint32
    Level     uint8
    HP        uint8
    State     uint16
}
```

Сами большие здания можно хранить отдельно:

```go
type Building struct {
    ID      uint64
    WorldID uint64
    X       int32
    Y       int32
    Type    uint16
    OwnerID uint64
    Level   uint8
    HP      uint16
    Storage ResourceStorage
}
```

Если здание занимает несколько клеток, у здания есть footprint, а в чанке можно хранить только “занятые клетки” или ссылку на главный tile.

## 4. Размеры

Возьмем нормальный вариант:

```text
Base  = 2 bytes / cell
Cover = 2 bytes / cell
Meta  = 1-2 bytes / cell
```

Итого примерно **5–6 байт на клетку** без зданий.

| Размер карты |    Клеток | Примерный raw size при 6 B/cell |
| -----------: | --------: | ------------------------------: |
|    `128x128` |    16 384 |                          ~96 KB |
|    `256x256` |    65 536 |                         ~384 KB |
|    `512x512` |   262 144 |                         ~1.5 MB |
|  `1024x1024` | 1 048 576 |                           ~6 MB |
|  `2048x2048` | 4 194 304 |                          ~24 MB |

То есть даже карта `1024x1024` для такой игры — это вообще не страшно. Страшно станет не от размера карты, а от истории ходов, логов, экономики, pathfinding и кривой модели объектов.

Для чата на 30 человек можно начинать с `128x128` или `192x192`. Для сезона с ростом чата — добавлять острова/континенты чанками.

## 5. Чанк `32x32` или `64x64`

Я бы выбрал **32x32**.

`32x32`:

```text
1024 клетки
Base:  2048 байт
Cover: 2048 байт
Meta:  1024-2048 байт
Итого: ~5-6 KB raw на чанк
```

Плюсы:

* удобно грузить/сохранять только измененные чанки;
* маленькие диффы;
* хорошая гранулярность для событий;
* проще отдавать куски карты в Telegram/веб-интерфейс;
* если игроки копаются в одном острове, не надо трогать всю карту.

`64x64` тоже окей, но один dirty chunk уже ~24 KB raw. Сжатие будет лучше, но обновления тяжелее.

## 6. Формат хранения

Я бы разделил на два слоя:

1. **логическая схема** — protobuf;
2. **плотные массивы карты** — `bytes` внутри protobuf.

Protocol Buffers хороши как формат со схемой: они компактнее JSON, быстрее, и генерируют Go-код по `.proto`-описанию. Но важный момент: не надо делать `repeated Cell cells = ...`, потому что у каждой клетки будет overhead полей. Лучше класть плотные бинарные массивы в `bytes`. ([Protobuf][1])

Пример `.proto`:

```proto
syntax = "proto3";

package game.world;

message ChunkData {
  int32 x = 1;
  int32 y = 2;

  uint32 version = 3;
  uint64 updated_tick = 4;

  bytes base = 10;   // 1024 * uint16 little-endian
  bytes cover = 11;  // 1024 * uint16 little-endian
  bytes meta = 12;   // 1024 * uint8/uint16

  repeated BuildingRef buildings = 20;
}

message BuildingRef {
  uint32 cell_index = 1;
  uint32 type = 2;
  uint64 owner_id = 3;
  uint32 level = 4;
  uint32 hp = 5;
  uint32 state = 6;
}
```

В Go для упаковки/распаковки массивов можно использовать `encoding/binary`; официальный пакет умеет переводить числа в последовательности байтов и обратно, включая fixed-size значения и varint’ы. ([Go.dev][2])

## 7. Сжатие: gzip или zstd?

Мой выбор: **zstd для чанков**, gzip только для совместимости/экспорта.

Go имеет стандартный `compress/gzip`, то есть gzip можно использовать без внешних зависимостей. ([Go.dev][3]) Но для игровых чанков я бы брал zstd через `klauspost/compress/zstd`: пакет pure Go, поддерживает компрессию/декомпрессию Zstandard и дает широкий компромисс скорость/степень сжатия. ([Go.dev][4])

Практически:

```text
Для live-сохранений: zstd level 1-3
Для backup/export:   zstd level 5-9 или gzip
Для сети:            zstd level 1
```

Карта из биомов, леса, воды и полей будет сжиматься очень хорошо, потому что рядом много похожих клеток. Условные `6 KB` чанка часто могут стать `1–3 KB`, особенно если карта сгенерирована естественными пятнами.

## 8. Где хранить

### Вариант А: один инстанс бота, простая игра

**bbolt** или **SQLite**.

bbolt — embedded key/value store для Go, подходит для простого локального хранения без отдельного Postgres/MySQL. ([GitHub][5])

Ключи:

```text
world:{worldID}:meta
world:{worldID}:chunk:{cx}:{cy}
world:{worldID}:building:{buildingID}
world:{worldID}:player:{playerID}
```

Значение чанка:

```text
zstd(protobuf(ChunkData))
```

Это очень простой и надежный вариант для MVP.

### Вариант B: хочешь нормальную серверную архитектуру

**PostgreSQL**.

Таблица чанков:

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

PostgreSQL `bytea` предназначен для хранения бинарных строк/байтов, включая нулевые и непечатаемые байты. ([PostgreSQL][6])

Таблица зданий/игроков/событий — обычными SQL-таблицами.

Мой выбор для серьезного бэка: **Postgres + compressed protobuf chunks в `bytea` + отдельные SQL-таблицы для игроков, владения, дипломатии, рынка и логов**.

### Вариант C: SQLite

SQLite тоже хранит BLOB “ровно как передан”, без текстовой интерпретации. ([SQLite][7]) Для одиночного сервера или локального MVP это тоже норм.

## 9. Что хранить в чанке, а что отдельно

В чанке:

```text
- biome
- soil
- height
- water/terrain flags
- forest/field/road/etc
- resource level
- local ruins/decor
- ссылки на маленькие static objects
```

Отдельно в БД:

```text
- игроки
- фракции/кланы
- владение территориями
- здания с инвентарем
- армии/юниты
- рынок
- события сезона
- дипломатия
- история ходов
- scheduled jobs
```

Важное правило: **в клетке не должно быть сложных сущностей**. Клетка — это поверхность мира. Все, что имеет владельца, инвентарь, таймер, производство или историю, лучше хранить как entity.

## 10. Рекомендуемая структура Go-пакетов

```text
/internal/world
    world.go
    chunk.go
    cellpack.go
    generation.go
    resources.go

/internal/storage
    storage.go
    postgres.go
    bbolt.go

/internal/codec
    chunk_codec.go
    compression.go

/internal/sim
    tick.go
    growth.go
    production.go
    decay.go

/internal/game
    commands.go
    actions.go
```

Интерфейс хранилища:

```go
type ChunkStore interface {
    LoadChunk(ctx context.Context, worldID uint64, cx, cy int32) (*world.Chunk, error)
    SaveChunk(ctx context.Context, worldID uint64, chunk *world.Chunk) error
    SaveChunks(ctx context.Context, worldID uint64, chunks []*world.Chunk) error
}
```

Кодек:

```go
type ChunkCodec interface {
    Encode(chunk *world.Chunk) ([]byte, error)
    Decode(data []byte) (*world.Chunk, error)
}
```

## 11. Dirty chunks

Не сохраняй всю карту после каждого действия.

```go
type World struct {
    ID     uint64
    Chunks map[ChunkCoord]*Chunk
    Dirty  map[ChunkCoord]struct{}
}
```

При действии:

```go
func (w *World) SetCover(x, y int32, cover CoverType, level uint8) {
    cx, cy, idx := ToChunkIndex(x, y)
    ch := w.MustChunk(cx, cy)

    ch.SetCover(idx, cover, level)
    ch.Dirty = true
    w.Dirty[ChunkCoord{cx, cy}] = struct{}{}
}
```

Потом периодически:

```go
func FlushDirty(ctx context.Context, store ChunkStore, w *World) error {
    chunks := make([]*Chunk, 0, len(w.Dirty))

    for coord := range w.Dirty {
        chunks = append(chunks, w.Chunks[coord])
    }

    if err := store.SaveChunks(ctx, w.ID, chunks); err != nil {
        return err
    }

    clear(w.Dirty)
    return nil
}
```

## 12. История и rollback

Для сезона я бы хранил не только текущее состояние, но и event log:

```sql
CREATE TABLE world_events (
    id BIGSERIAL PRIMARY KEY,
    world_id BIGINT NOT NULL,
    tick BIGINT NOT NULL,
    actor_id BIGINT,
    event_type TEXT NOT NULL,
    payload BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`payload` тоже protobuf.

Примеры событий:

```text
- PlayerJoined
- CellHarvested
- BuildingPlaced
- ForestGrown
- IslandDiscovered
- ResourceExhausted
- FloodStarted
```

Это даст:

* дебаг;
* replay сезона;
* rollback;
* статистику;
* красивые итоги сезона;
* защиту от багов.

Но не обязательно восстанавливать карту из event log каждый раз. Текущее состояние — чанки. Event log — история.

## 13. Рост чата во время сезона

Технически это ложится идеально:

```text
world grows by adding chunks
```

Не надо пересоздавать карту. Просто открываешь новые регионы.

Например:

```go
type Region struct {
    ID       uint64
    WorldID  uint64
    Kind     RegionKind // Continent, Island, Archipelago, Reef
    MinCX    int32
    MinCY    int32
    MaxCX    int32
    MaxCY    int32
    Unlocked bool
}
```

При росте чата:

```text
30 игроков  -> стартовый остров
50 игроков  -> архипелаг рядом
100 игроков -> новый континент
150 игроков -> "отлив" открыл земли
200 игроков -> северные острова / вулканический регион
```

С точки зрения хранения это просто новые чанки в таблице.

## 14. Мой конкретный стек для MVP

Я бы делал так:

```text
Language:      Go
Storage MVP:   PostgreSQL или bbolt
Format:        protobuf
Chunk payload: dense binary bytes
Compression:   zstd level 1-3
Map chunks:    32x32
Cell storage:  Base uint16 + Cover uint16 + Meta uint8/uint16
Entities:      отдельно в SQL/kv
History:       protobuf event log
```

Самый практичный вариант:

```text
Postgres:
- players
- factions
- buildings
- world_chunks(data bytea)
- world_events(payload bytea)
```

А в `world_chunks.data` лежит:

```text
zstd(protobuf(ChunkData))
```

## 15. Короткий вывод

Для твоей игры карта будет занимать смешные объемы. Даже `1024x1024` при нормальной упаковке — порядка **6 MB raw**, а после zstd может быть сильно меньше. Главная ошибка — не размер карты, а если начать хранить каждую клетку как жирный объект с владельцем, инвентарем, таймерами и JSON-полями.

Я бы начинал с:

```text
32x32 chunks
uint16 base
uint16 cover
uint8/uint16 meta
sparse buildings
protobuf wrapper
zstd compression
Postgres bytea или bbolt
```

смотри, а теперь давай обсудим, вот мы рубим дерево, получаем ресурсы. где их хранить? есть континент/mainland, есть несколько локальных поселений на нем. есть частные люди которые перемещаясь по клеткам собирают ресурсы в карман (или не в карман?). просто надо в карман, потому что тогда его можно грабить


Да, **в карман/личный груз надо**. Иначе пропадает целый слой игры: ограбления, караваны, охрана, риск выхода за ресурсами, засады, “пошел рубить лес — вернулся без штанов”.

Но я бы не делал так, что ресурс существует только в кармане. Лучше сделать **4 физических состояния ресурса**.

## Основная модель

Ресурс бывает здесь:

```text
1. В природе
   Лес в клетке, руда в горе, рыба в воде.

2. У персонажа
   Карман / рюкзак / руки / тележка.

3. На земле
   Куча бревен, мешок зерна, дроп после смерти, временный тайник.

4. В поселении
   Склад, амбар, казна, частный сундук, общинный склад.
```

А **континент/mainland не должен сам хранить ресурсы**. Континент — это регион, карта, политическая/географическая сущность. Ресурсы физически лежат в поселениях, у людей, на клетках или в караванах.

То есть не так:

```text
Mainland wood = 10000
```

А так:

```text
Поселение A: 300 дерева
Поселение B: 900 дерева
Игрок Вася: 12 дерева
Караван #17: 200 дерева
Клетка x=10 y=15: куча 40 дерева
```

А “на континенте всего 1452 дерева” — это просто агрегат для UI/статистики.

---

# Как происходит рубка дерева

Например клетка:

```text
x=42 y=18
cover = BirchForest
resource_level = 37
```

Игрок рубит дерево.

Дальше логика:

```text
1. Уменьшаем ресурс леса в клетке.
2. Создаем добытый ресурс Wood.
3. Пытаемся положить Wood в личный груз игрока.
4. Если не влезло — остаток падает на землю как GroundPile.
```

Пример:

```text
Игрок срубил 15 дерева.
Свободная грузоподъемность: 8.
8 дерева попало в личный груз.
7 дерева осталось кучей на клетке.
```

Вот эта куча на земле — очень вкусная механика. Ее можно:

```text
- украсть;
- охранять;
- потом забрать тележкой;
- сжечь;
- присвоить поселением;
- использовать как приманку.
```

---

# “Карман” лучше назвать личным грузом

Потому что дерево буквально в кармане звучит странно. Но механически это то же самое.

Я бы разделил личный инвентарь на несколько зон:

```text
Карман      — мелкие ценные вещи, деньги, семена, инструменты.
Рюкзак      — еда, немного дерева, травы, руда.
Руки/груз   — тяжелые бревна, камень, мешки.
Тележка     — отдельный переносимый контейнер.
```

Для MVP можно упростить:

```go
type InventoryKind uint8

const (
    InventoryPersonal InventoryKind = iota // личный груз игрока
    InventorySettlement                    // склад поселения
    InventoryGroundPile                    // куча на земле
    InventoryCaravan                       // караван/тележка
)
```

Но в геймдизайне уже считать, что `InventoryPersonal` — это не только карман, а “то, что человек несет на себе”.

---

# Важное правило: добытый ресурс — это entity, а не часть карты

Карта хранит **природное состояние**:

```text
лес есть / леса нет / сколько осталось древесины
```

А после добычи ресурс должен жить отдельно.

То есть в чанке:

```go
type CoverCell uint16 // forest type + resource level
```

А добытые ресурсы — в инвентарях:

```go
type ResourceKind uint16

const (
    ResourceWood ResourceKind = iota
    ResourceStone
    ResourceFood
    ResourceIronOre
    ResourceGold
)

type ResourceStack struct {
    Resource ResourceKind
    Amount   uint32
    Quality  uint8
}
```

---

# Универсальный контейнер инвентаря

Я бы сделал все хранилища через одну модель: **InventoryContainer**.

```go
type InventoryID uint64
type EntityID uint64

type InventoryKind uint8

const (
    InventoryPersonal InventoryKind = iota
    InventorySettlement
    InventoryBuilding
    InventoryGroundPile
    InventoryCaravan
    InventoryCorpse
)
```

```go
type Inventory struct {
    ID       InventoryID
    WorldID  uint64
    Kind     InventoryKind

    OwnerType OwnerType
    OwnerID   uint64

    Location  Location

    MaxWeight uint32
    MaxVolume uint32

    Flags     InventoryFlags
    UpdatedTick uint64
}
```

Локация:

```go
type LocationType uint8

const (
    LocationActor LocationType = iota
    LocationCell
    LocationSettlement
    LocationBuilding
    LocationCaravan
)

type Location struct {
    Type LocationType

    ActorID      uint64
    SettlementID uint64
    BuildingID   uint64

    X int32
    Y int32
}
```

Стек ресурсов:

```go
type InventoryStack struct {
    InventoryID InventoryID
    Resource    ResourceKind
    Amount      uint32
    Quality     uint8
}
```

Так у тебя и карман игрока, и склад поселения, и куча бревен на земле, и караван — это одна и та же система.

---

# В SQL это может выглядеть так

```sql
CREATE TABLE inventories (
    id BIGSERIAL PRIMARY KEY,
    world_id BIGINT NOT NULL,

    kind SMALLINT NOT NULL,

    owner_type SMALLINT NOT NULL,
    owner_id BIGINT,

    location_type SMALLINT NOT NULL,
    actor_id BIGINT,
    settlement_id BIGINT,
    building_id BIGINT,
    x INT,
    y INT,

    max_weight INT NOT NULL,
    max_volume INT NOT NULL,

    flags INT NOT NULL DEFAULT 0,
    updated_tick BIGINT NOT NULL
);
```

```sql
CREATE TABLE inventory_stacks (
    inventory_id BIGINT NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    resource SMALLINT NOT NULL,
    quality SMALLINT NOT NULL DEFAULT 0,
    amount BIGINT NOT NULL,

    PRIMARY KEY (inventory_id, resource, quality)
);
```

Для игры в чате этого хватит очень надолго.

---

# Как хранить склады поселений

У поселения может быть несколько инвентарей:

```text
1. Общий склад поселения
2. Казна
3. Военный склад
4. Частные сундуки игроков
5. Склад фракции/клана
```

Например:

```text
Settlement #5
- PublicWarehouse: дерево, камень, еда
- Treasury: монеты, редкие ресурсы
- Armory: оружие, инструменты
- PrivateChest: игрок Вася
- PrivateChest: игрок Петя
```

Для MVP можно начать с двух:

```text
- общий склад поселения;
- личный инвентарь игрока.
```

Потом добавить частные сундуки.

---

# Грабеж

Да, если ресурс в личном грузе — его можно грабить.

Но я бы ввел разные уровни доступности:

```text
Руки/груз       — почти всегда можно отнять.
Рюкзак          — можно ограбить после победы/засады.
Карман          — часть можно спрятать.
Склад           — нужен рейд/взлом/осада.
Казна           — нужна победа над поселением или доступ по роли.
Куча на земле   — можно украсть, если никто не охраняет.
```

Даже если в коде пока один `InventoryPersonal`, в правилах можно сделать так:

```text
Чем тяжелее ресурс, тем легче его отнять.
```

Например:

```text
Дерево, камень, руда — 80-100% лутается.
Еда — 50-80%.
Монеты — 30-70%.
Семена/артефакты — зависит от скрытности.
Инструменты — могут ломаться/отниматься.
```

---

# Ограничение грузоподъемности

Это важно, иначе игроки будут носить по 99999 дерева.

У каждого ресурса:

```go
type ResourceDef struct {
    Kind   ResourceKind
    Weight uint32
    Volume uint32
}
```

Например:

```text
1 дерево = вес 5, объем 4
1 камень = вес 8, объем 3
1 еда    = вес 1, объем 1
1 золото = вес 10, объем 1
```

У игрока:

```text
Без рюкзака: 20 веса
С рюкзаком: 40 веса
С тележкой: 150 веса
С повозкой: 500 веса
```

Тогда дерево становится логистическим ресурсом. Его можно добыть, но надо вывезти.

Это прям хорошо для игры.

---

# Почему не стоит делать глобальный склад континента

Если сделать:

```text
MainlandStorage
```

то логистика умрет. Игрок срубил дерево на севере, а оно магически появилось на юге. Это удобно, но скучно.

Лучше так:

```text
Континент не хранит ресурсы.
Поселения хранят ресурсы.
Между поселениями ресурсы возят люди/караваны.
```

Тогда появляются:

```text
- торговые пути;
- разбойники;
- охрана караванов;
- налоги;
- блокада поселений;
- голод;
- локальные дефициты;
- войны за лес рядом с городом.
```

Это намного интереснее.

---

# Караваны

Когда игроки хотят перенести много ресурсов, они создают караван.

```go
type Caravan struct {
    ID       uint64
    WorldID  uint64

    OwnerID  uint64
    FromID   uint64
    ToID     uint64

    X int32
    Y int32

    Speed uint16
    GuardPower uint32

    InventoryID uint64
}
```

Караван — это просто сущность с большим `Inventory`.

Его можно:

```text
- ограбить;
- сопроводить;
- задержать;
- потерять в болоте;
- обложить налогом;
- спрятать;
- атаковать во время войны.
```

---

# Временные кучи на земле

Я бы обязательно сделал `GroundPile`.

Когда игрок не может унести весь ресурс:

```text
На клетке появляется куча.
```

Она имеет:

```text
- координаты;
- ресурс;
- количество;
- владельца, если нужно;
- время исчезновения;
- флаг “охраняется/не охраняется”.
```

Например:

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

Можно сделать decay:

```text
бревна лежат долго;
еда портится;
руда лежит бесконечно;
деньги на земле быстро подбирают NPC/воры.
```

---

# Мой рекомендуемый flow

## Рубка дерева

```text
Игрок находится на клетке леса.
Игрок использует действие “рубить”.
Клетка теряет N древесины.
Создается N дерева.
Часть идет в personal inventory.
Остаток падает в ground pile.
```

## Возврат в поселение

```text
Игрок приходит в поселение.
Использует действие “сдать ресурсы”.
Ресурсы переходят из personal inventory в settlement warehouse.
```

## Ограбление

```text
Игрок А атакует игрока Б.
Если А победил:
- часть personal inventory игрока Б переходит игроку А;
- часть может упасть на землю;
- часть может быть уничтожена;
- тяжелый груз почти всегда теряется.
```

## Рейд на склад

```text
Группа атакует поселение.
Если пробили защиту:
- часть склада можно разграбить;
- часть сгорает;
- часть защищена стенами/гарнизоном/ролями.
```

---

# Важный геймдизайн: не все должно быть одинаково грабибельно

Я бы сделал ресурсы с параметрами:

```text
weight       — вес
volume       — объем
lootability  — насколько легко отнять
perishable   — портится ли
contraband   — незаконный/опасный ресурс
visibility   — видно ли, что игрок тащит ресурс
```

Например:

```text
Бревна:
- тяжелые
- заметные
- легко отнять
- трудно спрятать

Золото:
- тяжелое, но компактное
- легко спрятать
- ценно для грабителей

Еда:
- средний вес
- портится
- нужна поселениям

Семена:
- легкие
- ценные
- можно спрятать
```

Тогда “ограбить лесоруба” и “ограбить курьера с золотом” — разные события.

---

# Что делать с частными людьми

Я бы дал каждому игроку:

```text
1. Personal inventory
2. Private storage в родном поселении
3. Возможно, доступ к settlement storage
```

Пример:

```text
Вася:
- несет с собой: 12 дерева, 2 еды, 1 топор
- личный сундук: 30 еды, 3 железа
- вклад в общий склад поселения: 100 дерева
```

Общий склад — политика.

Можно сделать роли:

```text
Житель       — может сдавать ресурсы.
Кладовщик    — может выдавать ресурсы.
Староста     — может тратить склад на постройки.
Воевода      — может брать оружие.
Вор          — может попытаться украсть.
```

---

# Что я бы выбрал для MVP

Минимальная, но уже вкусная система:

```text
1. Ресурс в природе хранится в клетке.
2. После добычи ресурс попадает в personal inventory.
3. Если не влез — падает в ground pile.
4. В поселении можно сдать ресурс в warehouse.
5. Personal inventory можно грабить.
6. Warehouse можно грабить только рейдом.
7. Континент не имеет своего склада.
```

То есть:

```text
Cell resource -> Personal inventory -> Settlement warehouse
                         |
                         v
                    Ground pile
```

Это очень сильная основа.

---

# Коротко

Да, **надо делать карман/личный груз**, потому что это рождает грабежи, охрану, риск и социальные конфликты.

Но правильнее так:

```text
Дерево в лесу — часть клетки.
Срубленное дерево — физический ресурс.
Физический ресурс всегда лежит в конкретном контейнере:
- у игрока;
- на земле;
- в складе;
- в караване.
```

А `mainland` лучше оставить не складом, а регионом. Ресурсы должны быть локальными. Тогда игра получает логистику, дефицит, разбой, караваны, торговлю и войны за удобные места.


Карман      — мелкие ценные вещи, деньги, семена, инструменты.
Рюкзак      — еда, немного дерева, травы, руда.
Руки/груз   — тяжелые бревна, камень, мешки.
Тележка     — отдельный переносимый контейнер.

это перебор, хватит кармана (топор, деньги, карта), и тележки, это рюкзак но побольше, его можно грабить, возить, закрывать и открывать, ключ от нее в карманах
инструменты в карманах применются и выбираются при действии автоматически


Да, это сильно лучше для MVP и даже для финальной версии. Я бы оставил **ровно две переносимые сущности**:

```text
1. Карман персонажа
   Маленький, личный, всегда с собой.

2. Тележка
   Большая, физическая, стоит на клетке или тянется персонажем.
```

А “рюкзак”, “руки”, “мешки” — не нужны. Их можно симулировать через тележку и лимиты.

## Базовая модель

```text
Карман:
- топор
- нож
- деньги
- ключи
- карта
- семена
- мелкие редкие предметы
- документы / права / жетоны

Тележка:
- дерево
- камень
- руда
- еда
- строительные материалы
- добыча с леса/поля/шахты
```

Тогда добыча работает просто:

```text
Игрок рубит дерево.
Топор автоматически берется из кармана.
Дерево кладется в тележку.
Если тележки нет рядом — дерево падает на землю.
```

Это прям хорошая логика.

---

# Карман

Карман — это не грузовой инвентарь. Он для **инструментов, прав доступа и ценностей**.

```go
type Pocket struct {
    OwnerID uint64
    Items   []ItemStack
}
```

Или универсально:

```go
type InventoryKind uint8

const (
    InventoryPocket InventoryKind = iota
    InventoryCart
    InventoryGroundPile
    InventorySettlementStorage
)
```

Для кармана лучше задать ограничения не только по весу, но и по типам предметов.

Например:

```go
type ItemDef struct {
    ID       uint16
    Name     string
    Weight   uint16
    Volume   uint16
    Tags     ItemTags
}

type ItemTags uint32

const (
    TagTool ItemTags = 1 << iota
    TagKey
    TagMoney
    TagMap
    TagSmallValuable
    TagBulkResource
)
```

И правило:

```text
Карман принимает:
- tools
- keys
- money
- maps
- small valuables

Карман не принимает:
- дерево
- камень
- руду
- большие мешки еды
```

То есть нельзя положить `100 дерева` в карман, даже если вес позволяет.

---

# Тележка

Тележка — это отдельная entity на карте.

```go
type Cart struct {
    ID        uint64
    WorldID   uint64

    X int32
    Y int32

    OwnerID   uint64
    LockID    uint64

    IsLocked  bool
    IsAttached bool
    AttachedToActorID uint64

    InventoryID uint64

    HP        uint16
    CapacityWeight uint32
    CapacityVolume uint32
}
```

Смысл:

```text
- тележка стоит на клетке;
- игрок может взять ее и тянуть;
- тележку можно открыть;
- тележку можно закрыть;
- тележку можно взломать;
- тележку можно угнать;
- тележку можно разграбить;
- тележку можно потерять;
- тележку можно оставить у склада.
```

В коде тележка имеет обычный inventory:

```go
type Inventory struct {
    ID      uint64
    Kind    InventoryKind
    OwnerID uint64

    Location InventoryLocation

    MaxWeight uint32
    MaxVolume uint32
}
```

```go
type InventoryStack struct {
    InventoryID uint64
    ItemID      uint16
    Amount      uint32
    Quality     uint8
}
```

---

# Ключи

Ключ — это предмет в кармане.

```go
type KeyItem struct {
    ItemID uint64
    KeyID  uint64
}
```

А у тележки есть `LockID`.

```text
Если у игрока в кармане есть ключ с KeyID == Cart.LockID,
он может открыть/закрыть тележку.
```

Пример:

```go
func CanOpenCart(actor *Actor, cart *Cart) bool {
    if !cart.IsLocked {
        return true
    }

    return actor.Pocket.HasKey(cart.LockID)
}
```

Это дает классные ситуации:

```text
- убил лесоруба, забрал ключ, открыл тележку;
- украл ключ из кармана;
- потерял ключ — надо ломать замок;
- дал ключ союзнику;
- сделал дубликат ключа;
- тележку можно угнать, но без ключа нельзя открыть;
- можно тащить чужую закрытую тележку, но это заметное преступление.
```

Я бы разрешил **тащить закрытую чужую тележку**, но с последствиями. Это смешнее.

---

# Автовыбор инструментов из кармана

Да, это правильно. Игрок не должен каждый раз выбирать топор руками.

Действие:

```text
/рубить
```

Система сама ищет лучший подходящий инструмент в кармане:

```go
func SelectToolForAction(actor *Actor, action ActionKind) *ItemStack {
    candidates := actor.Pocket.FindToolsFor(action)

    if len(candidates) == 0 {
        return nil
    }

    return BestTool(candidates)
}
```

Например:

```text
Рубить лес:
- каменный топор
- железный топор
- стальной топор

Копать:
- палка-копалка
- лопата
- железная лопата

Жать поле:
- нож
- серп
- коса
```

Если у игрока несколько инструментов, выбирается лучший:

```text
1. подходящий под действие;
2. не сломанный;
3. с максимальной эффективностью;
4. при равенстве — самый дешевый/старый, чтобы не тратить редкий.
```

Можно сделать параметр:

```go
type ToolDef struct {
    ItemID      uint16
    ActionMask  ActionMask
    Efficiency  uint16
    DurabilityCost uint16
}
```

---

# Рубка дерева с тележкой

Я бы сделал так:

```text
1. Игрок стоит на клетке с лесом.
2. Игрок пишет /рубить.
3. Система ищет топор в кармане.
4. Если топора нет — нельзя рубить или рубит очень медленно.
5. Система ищет доступную тележку:
   - прицепленную к игроку;
   - или открытую свою тележку на этой же клетке;
   - или открытую союзную тележку.
6. Добытое дерево кладется в тележку.
7. Если тележки нет или она заполнена — дерево падает кучей на землю.
```

То есть:

```text
лесная клетка -> действие -> тележка
                          -> куча на земле, если тележки нет
```

Карман тут не участвует как хранилище дерева.

---

# Ground pile все равно нужен

Даже при наличии тележек нужна куча на земле.

Потому что:

```text
- тележка заполнена;
- тележку украли;
- игрок умер;
- ресурс выбросили;
- склад разрушили;
- при грабеже часть выпала;
- добывали без тележки.
```

`GroundPile` — это просто временный контейнер на клетке.

```go
type GroundPile struct {
    ID          uint64
    WorldID     uint64
    X           int32
    Y           int32
    InventoryID uint64
    CreatedTick uint64
    DecayTick   uint64
}
```

---

# Грабеж

С такой системой грабеж становится очень понятным.

## Грабеж персонажа

Из кармана можно забрать:

```text
- деньги;
- ключи;
- карту;
- инструменты;
- редкие мелочи.
```

Но карман маленький, поэтому это не источник огромной добычи.

## Грабеж тележки

Вот тут основная добыча:

```text
- дерево;
- руда;
- еда;
- камень;
- стройматериалы.
```

Если тележка открыта — можно просто забрать.
Если закрыта — нужен ключ или взлом.

## Угон тележки

Можно даже не вскрывать, а просто угнать:

```text
Разбойники не открыли тележку, но увели ее в лес.
```

Потом у них варианты:

```text
- взломать;
- найти ключ;
- продать закрытую;
- требовать выкуп;
- привести к своему поселению и вскрыть там.
```

Это очень социально.

---

# Важный момент: тележка должна замедлять

Чтобы тележка была не просто “большой рюкзак”, у нее должны быть минусы:

```text
Без тележки:
- быстро ходишь;
- можешь скрываться;
- мало несешь.

С тележкой:
- медленнее ходишь;
- хуже проходишь лес/болото/горы;
- тебя видно;
- тебя легче догнать;
- зато много несешь.
```

Пример правил:

```text
Обычный ход: 3 клетки за тик.
С тележкой: 1-2 клетки за тик.
По дороге: +1 клетка.
По болоту с тележкой: нельзя или шанс застрять.
```

Это сразу делает дороги, охрану и инфраструктуру важными.

---

# Открыть / закрыть / доступ

Для тележки можно сделать простую модель доступа:

```go
type LockState uint8

const (
    LockNone LockState = iota
    LockOpen
    LockClosed
    LockBroken
)
```

Но я бы не делал `Open/Closed` отдельно от `Locked`. Лучше так:

```text
Открыта:
- любой рядом может брать ресурсы.

Закрыта:
- брать может только тот, у кого есть ключ.
- остальные должны взламывать.

Сломана:
- больше нельзя закрыть, пока не починят.
```

Команды:

```text
/тележка открыть
/тележка закрыть
/тележка прицепить
/тележка отцепить
/тележка взять 20 дерево
/тележка положить 10 еда
```

Для Telegram можно упростить кнопками.

---

# Частная собственность

У тележки должен быть владелец, но владение не должно магически запрещать действия.

```text
OwnerID нужен не для запрета.
OwnerID нужен для законов, репутации и последствий.
```

То есть чужую тележку можно тронуть физически, но игра запоминает:

```text
Петя угнал тележку Васи.
Петя украл 40 дерева из тележки Васи.
Петя взломал замок тележки поселения Северное.
```

И дальше:

```text
- штраф;
- объявление преступником;
- месть;
- автоматическое сообщение в чат;
- потеря репутации;
- право на ответный грабеж.
```

Это лучше, чем “нельзя, потому что не твое”.

---

# Минимальная схема для Go

Я бы сделал так:

```go
type Actor struct {
    ID        uint64
    WorldID   uint64
    X, Y      int32

    PocketInventoryID uint64
    AttachedCartID    uint64
}
```

```go
type Cart struct {
    ID        uint64
    WorldID   uint64

    X, Y      int32
    OwnerID   uint64

    InventoryID uint64

    LockID    uint64
    LockState LockState

    AttachedToActorID uint64

    HP uint16
}
```

```go
type Inventory struct {
    ID      uint64
    WorldID uint64
    Kind    InventoryKind

    MaxWeight uint32
    MaxVolume uint32
}
```

```go
type InventoryStack struct {
    InventoryID uint64
    ItemID      uint16
    Amount      uint32
    Quality     uint8
}
```

```go
type ItemDef struct {
    ID     uint16
    Kind   ItemKind
    Tags   ItemTags

    Weight uint16
    Volume uint16

    MaxStack uint32
}
```

---

# Ключевой flow

```text
Игрок:
- имеет карман;
- носит ключи и инструменты;
- может прицепить одну тележку;
- может открыть тележку ключом;
- может добывать ресурс инструментом из кармана;
- добыча идет в тележку;
- без тележки добыча падает на землю.
```

Вот это прям чистая и крепкая модель.

---

# Что я бы выбрал как правило

Самое удачное:

```text
Карман не принимает bulk-ресурсы.
Тележка принимает bulk-ресурсы.
Инструменты всегда в кармане.
Ключи всегда в кармане.
Действия автоматически используют инструменты из кармана.
Тележка — физический контейнер, который можно закрыть, открыть, угнать, взломать и разграбить.
```

И короткий вариант для архитектуры:

```text
Pocket = маленький личный inventory игрока.
Cart = отдельная movable entity с inventory.
Key = item в Pocket.
Lock = ID на Cart.
Tool = item в Pocket, auto-selected by action.
Bulk resources = только Cart / GroundPile / Storage.
```

Это проще, понятнее игрокам и дает больше конфликтов, чем сложный инвентарь с руками/рюкзаком/карманом.

