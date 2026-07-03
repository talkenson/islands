package mapgen

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"islands/internal/world"
)

const (
	coverFlagRock uint8 = 1 << iota
	coverFlagMountain

	forcedMountainHeight = 0.91
	highlandHeight       = 0.74
	marshMaxHeight       = 0.56
	desertMaxHeight      = 0.64
)

func Generate(config Config) (*Map, error) {
	m, _, err := GenerateWithReport(config)
	return m, err
}

func GenerateWithReport(config Config) (*Map, *GenerateReport, error) {
	if config.Width <= 0 || config.Height <= 0 {
		return nil, nil, errInvalidSize
	}

	report := &GenerateReport{}
	stageStart := time.Now()
	recordStage := func(name string) {
		now := time.Now()
		report.Stages = append(report.Stages, StageTiming{Name: name, Duration: now.Sub(stageStart)})
		stageStart = now
	}

	rand := newRandom(config.Seed)
	shallowNoise := newValueNoise(makeSeed(config.Seed, "shallow"))
	riverNoise := newValueNoise(makeSeed(config.Seed, "river"))

	continents := createContinents(config, rand)
	recordStage("continents")
	result := &Map{
		Width:      config.Width,
		Height:     config.Height,
		Config:     config,
		Continents: continents,
		Chunks:     make(map[world.ChunkCoord]*world.Chunk),
		heights:    make([]float64, config.Width*config.Height),
	}

	chunks := initializeChunks(result, config)
	recordStage("initialize chunks")
	generateCells(result, config, continents, chunks)
	recordStage("generate cells")

	classifyWaterBodies(result, config)
	recordStage("classify water")
	addShallowWater(result, config, shallowNoise)
	recordStage("shallow water")
	addGeology(result, config, chunks)
	recordStage("geology")
	addRivers(result, config, chunks, rand, riverNoise)
	recordStage("rivers")
	clearDirty(result)
	result.Stats = collectStats(result)
	recordStage("stats")

	return result, report, nil
}

type invalidSizeError struct{}

func (invalidSizeError) Error() string { return "map dimensions must be positive" }

var errInvalidSize error = invalidSizeError{}

func initializeChunks(m *Map, config Config) []world.ChunkCoord {
	chunkWidth := (config.Width + world.ChunkSize - 1) / world.ChunkSize
	chunkHeight := (config.Height + world.ChunkSize - 1) / world.ChunkSize
	chunks := make([]world.ChunkCoord, 0, chunkWidth*chunkHeight)
	for cy := 0; cy < chunkHeight; cy++ {
		for cx := 0; cx < chunkWidth; cx++ {
			coord := world.ChunkCoord{X: int32(cx), Y: int32(cy)}
			m.Chunks[coord] = world.NewChunk(coord.X, coord.Y)
			chunks = append(chunks, coord)
		}
	}
	return chunks
}

func generateCells(m *Map, config Config, continents []Continent, chunks []world.ChunkCoord) {
	runChunkWorkerPool(config, chunks, func(jobs <-chan world.ChunkCoord) {
		heightNoise := newValueNoise(makeSeed(config.Seed, "height"))
		detailNoise := newValueNoise(makeSeed(config.Seed, "detail"))
		moistureNoise := newValueNoise(makeSeed(config.Seed, "moisture"))
		temperatureNoise := newValueNoise(makeSeed(config.Seed, "temperature"))
		forestNoise := newValueNoise(makeSeed(config.Seed, "forest"))
		lakeNoise := newValueNoise(makeSeed(config.Seed, "lake"))

		for coord := range jobs {
			x0, y0, x1, y1 := chunkBounds(config, coord)
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					createCell(m, x, y, config, continents, heightNoise, detailNoise, moistureNoise, temperatureNoise, forestNoise, lakeNoise)
				}
			}
		}
	})
}

func runChunkWorkers(config Config, chunks []world.ChunkCoord, work func(world.ChunkCoord)) {
	runChunkWorkerPool(config, chunks, func(jobs <-chan world.ChunkCoord) {
		for coord := range jobs {
			work(coord)
		}
	})
}

func runChunkWorkerPool(config Config, chunks []world.ChunkCoord, work func(<-chan world.ChunkCoord)) {
	workers := workerCount(config)
	if workers <= 1 || len(chunks) <= 1 {
		jobs := make(chan world.ChunkCoord, len(chunks))
		for _, chunk := range chunks {
			jobs <- chunk
		}
		close(jobs)
		work(jobs)
		return
	}

	jobs := make(chan world.ChunkCoord)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			work(jobs)
		}()
	}
	for _, coord := range chunks {
		jobs <- coord
	}
	close(jobs)
	wg.Wait()
}

func workerCount(config Config) int {
	if config.Workers > 0 {
		return config.Workers
	}
	return min(max(runtime.GOMAXPROCS(0), 1), 8)
}

func chunkBounds(config Config, coord world.ChunkCoord) (int, int, int, int) {
	x0 := int(coord.X) * world.ChunkSize
	y0 := int(coord.Y) * world.ChunkSize
	x1 := min(x0+world.ChunkSize, config.Width)
	y1 := min(y0+world.ChunkSize, config.Height)
	return x0, y0, x1, y1
}

func createContinents(config Config, rand *random) []Continent {
	marginX := float64(config.Width) * config.OceanMargin
	marginY := float64(config.Height) * config.OceanMargin
	minDimension := float64(min(config.Width, config.Height))
	continents := make([]Continent, 0, config.ContinentCount)

	for i := 0; i < config.ContinentCount; i++ {
		var best Continent
		bestScore := math.Inf(-1)
		for attempt := 0; attempt < config.ContinentPlacementAttempts; attempt++ {
			candidate := createContinentCandidate(i, config, rand, marginX, marginY, minDimension)
			score := getContinentDistanceScore(candidate, continents, config, minDimension)
			if score > bestScore {
				best = candidate
				bestScore = score
			}
			if score >= 1 {
				break
			}
		}
		continents = append(continents, best)
	}

	return continents
}

func createContinentCandidate(id int, config Config, rand *random, marginX, marginY, minDimension float64) Continent {
	countPressure := clamp(1/math.Sqrt(float64(config.ContinentCount)), 0.42, 1)
	radiusBase := minDimension * rand.rangeFloat(0.16, 0.29) * (0.72 + countPressure*0.34)

	continent := Continent{
		ID:       id,
		X:        rand.rangeFloat(marginX+radiusBase*0.55, float64(config.Width)-marginX-radiusBase*0.55),
		Y:        rand.rangeFloat(marginY+radiusBase*0.55, float64(config.Height)-marginY-radiusBase*0.55),
		RX:       radiusBase * rand.rangeFloat(0.85, 1.35),
		RY:       radiusBase * rand.rangeFloat(0.75, 1.25),
		Strength: rand.rangeFloat(0.88, 1.14),
		Wobble:   rand.rangeFloat(0.18, 0.34),
		Lobes:    createContinentLobes(rand),
	}
	continent.LakeBasins = createLakeBasins(continent, config, rand, minDimension)
	return continent
}

func createContinentLobes(rand *random) []Lobe {
	count := rand.int(3, 5)
	lobes := make([]Lobe, 0, count)
	for i := 0; i < count; i++ {
		lobes = append(lobes, Lobe{
			Frequency: float64(i) + rand.rangeFloat(2.2, 4.8),
			Amplitude: rand.rangeFloat(0.035, 0.11),
			Phase:     rand.rangeFloat(0, math.Pi*2),
		})
	}
	return lobes
}

func createLakeBasins(continent Continent, config Config, rand *random, minDimension float64) []LakeBasin {
	count := rand.int(config.LakeBasinsPerContinentMin, config.LakeBasinsPerContinentMax)
	basins := make([]LakeBasin, 0, count)
	for i := 0; i < count; i++ {
		radius := minDimension * rand.rangeFloat(config.LakeBasinMinRadius, config.LakeBasinMaxRadius)
		basins = append(basins, LakeBasin{
			X:     continent.X + rand.rangeFloat(-continent.RX*0.36, continent.RX*0.36),
			Y:     continent.Y + rand.rangeFloat(-continent.RY*0.36, continent.RY*0.36),
			RX:    radius * rand.rangeFloat(0.85, 1.65),
			RY:    radius * rand.rangeFloat(0.75, 1.35),
			Depth: rand.rangeFloat(0.24, 0.48),
		})
	}
	return basins
}

func getContinentDistanceScore(candidate Continent, continents []Continent, config Config, minDimension float64) float64 {
	if len(continents) == 0 {
		return 1
	}
	requiredGap := minDimension * config.ContinentSpacing
	weakest := math.Inf(1)
	for _, continent := range continents {
		requiredDistance := estimatedLandRadius(candidate) + estimatedLandRadius(continent) + requiredGap
		actualDistance := math.Hypot(candidate.X-continent.X, candidate.Y-continent.Y)
		weakest = math.Min(weakest, actualDistance/requiredDistance)
	}
	return weakest
}

func estimatedLandRadius(continent Continent) float64 {
	return math.Max(continent.RX, continent.RY) * 0.72
}

func createCell(m *Map, x, y int, config Config, continents []Continent, heightNoise, detailNoise, moistureNoise, temperatureNoise, forestNoise, lakeNoise *valueNoise) {
	continentInfluence := getContinentInfluence(float64(x), float64(y), continents, config, detailNoise)
	edgeFalloff := getEdgeFalloff(float64(x), float64(y), config)
	roughHeight := heightNoise.octaveNoise2D(float64(x)*config.HeightScale, float64(y)*config.HeightScale, 5, 0.53)
	basinDepth := getLakeBasinDepth(float64(x), float64(y), continentInfluence, edgeFalloff, continents, config, lakeNoise)
	height := clamp((continentInfluence*0.88+roughHeight*0.22-basinDepth)*edgeFalloff, 0, 1)
	moisture := clamp(moistureNoise.octaveNoise2D(float64(x)*config.MoistureScale, float64(y)*config.MoistureScale, 4, 0.56), 0, 1)
	latitude := math.Abs(float64(y)/float64(config.Height)-0.5) * 2
	rawTemperature := clamp(temperatureNoise.octaveNoise2D(float64(x)*config.TemperatureScale, float64(y)*config.TemperatureScale, 4, 0.5)*0.48+(1-latitude)*0.52, 0, 1)
	temperature := effectiveTemperature(rawTemperature, height, config)

	if height <= config.LandThreshold {
		m.setHeight(x, y, height)
		setCell(m, x, y, world.PackBase(world.BiomeCoast, world.SoilWater, uint8(height*31), 0), world.PackWater(world.WaterSea, 4, false), world.PackCover(world.CoverNone, 0, 0), 0)
		setTemperature(m, x, y, temperature)
		return
	}

	m.setHeight(x, y, height)
	biome, soil := chooseBiome(height, moisture, temperature, config)
	cover := world.CoverGrass
	level := uint8(1 + math.Round(moisture*2))
	stock := uint16(0)
	if forestKind, density, ok := createForest(x, y, moisture, temperature, biome, forestNoise, config); ok {
		cover = forestKind
		level = uint8(clamp(math.Round(1+density*4), 1, 5))
		stock = uint16(math.Round(6 + density*18))
	}

	setCell(m, x, y, world.PackBase(biome, soil, uint8(height*31), 0), world.PackWater(world.WaterNone, 0, false), world.PackCover(cover, level, 0), stock)
	setTemperature(m, x, y, temperature)
}

func getLakeBasinDepth(x, y, continentInfluence, edgeFalloff float64, continents []Continent, config Config, lakeNoise *valueNoise) float64 {
	if continentInfluence < config.LakeMinContinentInfluence || edgeFalloff < 0.98 {
		return 0
	}
	broadBasin := lakeNoise.octaveNoise2D(x*config.LakeScale, y*config.LakeScale, 4, 0.56)
	localBasin := lakeNoise.octaveNoise2D(x*config.LakeScale*2.4+37, y*config.LakeScale*2.4+53, 3, 0.5)
	basin := broadBasin*0.75 + localBasin*0.25
	basinAmount := clamp((basin-config.LakeThreshold)/(1-config.LakeThreshold), 0, 1)

	carvedDepth := 0.0
	for _, continent := range continents {
		for _, basin := range continent.LakeBasins {
			dx := (x - basin.X) / basin.RX
			dy := (y - basin.Y) / basin.RY
			radial := math.Sqrt(dx*dx + dy*dy)
			if radial < 1 {
				carvedDepth = math.Max(carvedDepth, math.Pow(1-radial, 1.45)*basin.Depth)
			}
		}
	}

	return basinAmount*basinAmount*config.LakeDepth + carvedDepth
}

func getContinentInfluence(x, y float64, continents []Continent, config Config, detailNoise *valueNoise) float64 {
	primary := 0.0
	secondary := 0.0

	for _, continent := range continents {
		dx := (x - continent.X) / continent.RX
		dy := (y - continent.Y) / continent.RY
		radial := math.Sqrt(dx*dx + dy*dy)
		angle := math.Atan2(dy, dx)
		angularNoise := detailNoise.octaveNoise2D(math.Cos(angle)*2.3+float64(continent.ID)*13, math.Sin(angle)*2.3+float64(continent.ID)*17, 4, 0.58) - 0.5
		lobeOffset := 0.0
		for _, lobe := range continent.Lobes {
			lobeOffset += math.Sin(angle*lobe.Frequency+lobe.Phase) * lobe.Amplitude
		}
		coastDetail := detailNoise.octaveNoise2D(x*config.CoastDetailScale+float64(continent.ID)*23, y*config.CoastDetailScale+float64(continent.ID)*29, 3, 0.62) - 0.5
		coastBand := 1 - clamp(math.Abs(radial-0.78)/0.58, 0, 1)
		shapeOffset := angularNoise*continent.Wobble + lobeOffset*config.ContinentRoughness + coastDetail*config.ContinentRoughness*coastBand
		noisyRadius := radial - shapeOffset
		local := clamp(1-noisyRadius, 0, 1)
		influence := math.Pow(local, 0.78) * continent.Strength
		if influence > primary {
			secondary = primary
			primary = influence
		} else if influence > secondary {
			secondary = influence
		}
	}

	return clamp(primary-secondary*config.ContinentSeparationStrength, 0, 1)
}

func getEdgeFalloff(x, y float64, config Config) float64 {
	dx := math.Min(x, float64(config.Width-1)-x) / (float64(config.Width) * config.OceanMargin)
	dy := math.Min(y, float64(config.Height-1)-y) / (float64(config.Height) * config.OceanMargin)
	return clamp(math.Min(dx, dy), 0, 1)
}

func effectiveTemperature(rawTemperature, height float64, config Config) float64 {
	elevation := clamp((height-config.LandThreshold)/(1-config.LandThreshold), 0, 1)
	return clamp(rawTemperature-elevation*0.32, 0, 1)
}

func chooseBiome(height, moisture, temperature float64, config Config) (world.Biome, world.Soil) {
	if height >= forcedMountainHeight {
		return world.BiomeMountain, world.SoilRocky
	}
	if height >= highlandHeight {
		if temperature < 0.38 || moisture > 0.58 {
			return world.BiomeTaiga, world.SoilRocky
		}
		return world.BiomeMeadow, world.SoilRocky
	}
	if height >= config.MountainHeight && temperature < 0.45 {
		return world.BiomeTaiga, world.SoilRocky
	}
	if height <= marshMaxHeight && moisture > 0.68 && temperature < 0.72 {
		return world.BiomeMarsh, world.SoilMarsh
	}
	if height <= desertMaxHeight && moisture < 0.26 && temperature > 0.62 {
		return world.BiomeDesert, world.SoilSand
	}
	if temperature < 0.34 && moisture > 0.36 {
		return world.BiomeTaiga, world.SoilGrass
	}
	if moisture < 0.42 {
		return world.BiomeSteppe, world.SoilGrass
	}
	if moisture < 0.58 {
		return world.BiomeMeadow, world.SoilGrass
	}
	if temperature < 0.48 {
		return world.BiomeTaiga, world.SoilGrass
	}
	if moisture > 0.72 {
		return world.BiomeTemperateForest, world.SoilFertile
	}
	return world.BiomeBirchForest, world.SoilGrass
}

func createForest(x, y int, moisture, temperature float64, biome world.Biome, forestNoise *valueNoise, config Config) (world.CoverKind, float64, bool) {
	cluster := forestNoise.octaveNoise2D(float64(x)*config.ForestClusterScale, float64(y)*config.ForestClusterScale, 4, 0.62)
	treeChance := 0.2
	moistureBias := 0.05
	switch biome {
	case world.BiomeTaiga:
		treeChance = 0.26
		moistureBias = 0.08
	case world.BiomeBirchForest:
		treeChance = 0.28
		moistureBias = 0.08
	case world.BiomeTemperateForest:
		treeChance = 0.24
		moistureBias = 0.12
	case world.BiomeMeadow:
		treeChance = 0.055
		moistureBias = 0.1
	case world.BiomeSteppe:
		treeChance = 0.02
		moistureBias = -0.25
	case world.BiomeMarsh:
		treeChance = 0.08
		moistureBias = 0.25
	case world.BiomeDesert, world.BiomeMountain:
		treeChance = 0.006
		moistureBias = -0.2
	}
	chance := treeChance + moistureBias*(moisture-0.5)
	threshold := clamp(0.78-clamp(chance, 0.005, 0.45)*1.1, 0.42, 0.82)
	if cluster < threshold {
		return world.CoverNone, 0, false
	}
	density := clamp((cluster-threshold)/(1-threshold), 0.28, 1)
	if biome == world.BiomeDesert || biome == world.BiomeSteppe || biome == world.BiomeMountain {
		return world.CoverDryBush, density, true
	}
	if biome == world.BiomeMarsh {
		if cluster > 0.94 {
			return world.CoverMixedForest, density * 0.65, true
		}
		return world.CoverReeds, density, true
	}
	if biome == world.BiomeTaiga {
		return world.CoverPineForest, density, true
	}
	if biome == world.BiomeBirchForest {
		return world.CoverBirchForest, density, true
	}
	if biome == world.BiomeMeadow {
		if temperature < 0.42 {
			return world.CoverPineForest, density * 0.8, true
		}
		return world.CoverBirchForest, density * 0.8, true
	}
	if cluster > 0.92 {
		return world.CoverMixedForest, density, true
	}
	if temperature < 0.46 {
		return world.CoverPineForest, density, true
	}
	return world.CoverBirchForest, density, true
}

func addShallowWater(m *Map, config Config, shallowNoise *valueNoise) {
	type queueItem struct {
		x, y     int
		distance int
		limit    int
	}
	var queue []queueItem
	bestRemaining := make(map[[2]int]int)

	for y := 0; y < config.Height; y++ {
		for x := 0; x < config.Width; x++ {
			if !isWater(m, x, y) || hasLandNeighbor(m, x, y) == false {
				continue
			}
			limit := localShallowWidth(m, x, y, config, shallowNoise)
			if limit <= 0 {
				continue
			}
			queue = append(queue, queueItem{x: x, y: y, distance: 1, limit: limit})
			bestRemaining[[2]int{x, y}] = limit - 1
		}
	}

	for i := 0; i < len(queue); i++ {
		item := queue[i]
		markShallow(m, item.x, item.y, item.distance)
		if item.distance >= item.limit {
			continue
		}
		for _, nb := range neighbors(item.x, item.y, config.Width, config.Height) {
			if !isWater(m, nb[0], nb[1]) {
				continue
			}
			nextDistance := item.distance + 1
			remaining := item.limit - nextDistance
			key := [2]int{nb[0], nb[1]}
			if remaining < 0 || bestRemaining[key] >= remaining {
				continue
			}
			bestRemaining[key] = remaining
			queue = append(queue, queueItem{x: nb[0], y: nb[1], distance: nextDistance, limit: item.limit})
		}
	}
}

func localShallowWidth(m *Map, x, y int, config Config, shallowNoise *valueNoise) int {
	broadShelf := shallowNoise.octaveNoise2D(float64(x)*config.ShallowWaterScale, float64(y)*config.ShallowWaterScale, 4, 0.58)
	brokenEdge := shallowNoise.noise2D(float64(x)*config.ShallowWaterScale*3+17, float64(y)*config.ShallowWaterScale*3+29)
	shelf := broadShelf*0.76 + brokenEdge*0.24
	if shelf < 0.3 {
		return 0
	}
	normalized := clamp((shelf-0.3)/0.52, 0, 1)
	maxWidth := config.ShallowWaterMaxWidthMin + int(math.Round(normalized*float64(config.ShallowWaterMaxWidthMax-config.ShallowWaterMaxWidthMin)))
	ch, idx := chunkCell(m, x, y)
	if ch.WaterCell(idx).Kind() == world.WaterLake {
		return max(config.ShallowWaterMinWidth, min(3, int(math.Round(float64(maxWidth)*0.45))))
	}
	return max(config.ShallowWaterMinWidth, maxWidth)
}

func addGeology(m *Map, config Config, chunks []world.ChunkCoord) {
	runChunkWorkerPool(config, chunks, func(jobs <-chan world.ChunkCoord) {
		geologyNoise := newValueNoise(makeSeed(config.Seed, "geology"))
		for coord := range jobs {
			x0, y0, x1, y1 := chunkBounds(config, coord)
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					ch, idx := chunkCell(m, x, y)
					if ch.WaterCell(idx).Kind() != world.WaterNone {
						continue
					}
					base := ch.BaseCell(idx)
					height := m.heightAt(x, y)
					if hasWaterNeighbor(m, x, y) {
						beachNoise := geologyNoise.octaveNoise2D(float64(x)*config.RockScale*1.8+71, float64(y)*config.RockScale*1.8+83, 3, 0.55)
						if beachNoise < config.RockyBeachChance {
							ch.SetBase(idx, world.PackBase(world.BiomeCoast, world.SoilRocky, base.Elevation(), base.Flags()))
							ch.SetCover(idx, world.PackCover(world.CoverNone, 0, coverFlagRock))
							ch.SetStock(idx, uint16(math.Round(2+beachNoise*8)))
							continue
						}
						if height < config.LandThreshold+0.08 {
							ch.SetBase(idx, world.PackBase(world.BiomeCoast, world.SoilSand, base.Elevation(), base.Flags()))
							ch.SetCover(idx, world.PackCover(world.CoverNone, 0, 0))
							ch.SetStock(idx, 0)
							continue
						}
					}
					mountainNoise := geologyNoise.octaveNoise2D(float64(x)*config.MountainScale+101, float64(y)*config.MountainScale+109, 4, 0.6)
					if base.Biome() == world.BiomeMountain || height >= forcedMountainHeight || (height > config.MountainHeight && mountainNoise > config.MountainThreshold) {
						ch.SetBase(idx, world.PackBase(world.BiomeMountain, world.SoilRocky, base.Elevation(), base.Flags()))
						ch.SetCover(idx, world.PackCover(world.CoverNone, 0, coverFlagMountain))
						ch.SetStock(idx, uint16(math.Round(18+(height-config.MountainHeight)*90)))
						continue
					}
					rockNoise := geologyNoise.octaveNoise2D(float64(x)*config.RockScale, float64(y)*config.RockScale, 4, 0.58)
					if rockNoise > config.RockThreshold && height > config.LandThreshold+0.07 {
						ch.SetBase(idx, world.PackBase(base.Biome(), world.SoilRocky, base.Elevation(), base.Flags()))
						ch.SetCover(idx, world.PackCover(world.CoverNone, 0, coverFlagRock))
						ch.SetStock(idx, uint16(math.Round(5+(rockNoise-config.RockThreshold)*35)))
					}
				}
			}
		}
	})
}

type riverCandidate struct {
	x, y   int
	height float64
}

func addRivers(m *Map, config Config, chunks []world.ChunkCoord, rand *random, riverNoise *valueNoise) {
	waterDistance := buildWaterDistanceField(m)
	candidates := collectRiverCandidates(m, config, chunks, waterDistance)
	sortRiverCandidates(candidates)

	targetCount := max(1, int(math.Round(float64(config.RiverCount))))
	attempts := 0
	created := 0
	for created < targetCount && len(candidates) > 0 && attempts < targetCount*12 {
		attempts++
		index := min(len(candidates)-1, int(math.Floor(math.Pow(rand.next(), 2.2)*float64(len(candidates)))))
		start := candidates[index]
		candidates = append(candidates[:index], candidates[index+1:]...)
		if isRiver(m, start.x, start.y) || nearbyRiver(m, start.x, start.y, 8) {
			continue
		}

		path := traceRiver(m, start, config, riverNoise, waterDistance, created)
		if len(path) >= config.MinRiverLength {
			end := path[len(path)-1]
			if isWater(m, end[0], end[1]) {
				paintRiverPath(m, path, config, riverNoise, created)
				created++
			}
		}
	}
}

func collectRiverCandidates(m *Map, config Config, chunks []world.ChunkCoord, waterDistance []uint16) []riverCandidate {
	candidates := make([]riverCandidate, 0)
	var mu sync.Mutex
	runChunkWorkerPool(config, chunks, func(jobs <-chan world.ChunkCoord) {
		local := make([]riverCandidate, 0)
		for coord := range jobs {
			x0, y0, x1, y1 := chunkBounds(config, coord)
			x0 = max(x0, 2)
			y0 = max(y0, 2)
			x1 = min(x1, config.Width-2)
			y1 = min(y1, config.Height-2)
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					ch, idx := chunkCell(m, x, y)
					if ch.WaterCell(idx).Kind() != world.WaterNone {
						continue
					}
					height := m.heightAt(x, y)
					if height < config.LandThreshold+0.15 {
						continue
					}
					if waterDistanceAt(m, waterDistance, x, y) > 8 {
						local = append(local, riverCandidate{x: x, y: y, height: height})
					}
				}
			}
		}
		if len(local) == 0 {
			return
		}
		mu.Lock()
		candidates = append(candidates, local...)
		mu.Unlock()
	})
	return candidates
}

func buildWaterDistanceField(m *Map) []uint16 {
	maxDistance := maxWaterDistance(m)
	distances := make([]uint16, m.Width*m.Height)
	for i := range distances {
		distances[i] = uint16(maxDistance)
	}

	queue := make([]int32, 0)
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			if !isWater(m, x, y) {
				continue
			}
			distances[y*m.Width+x] = uint16(0)
			if hasLandNeighbor(m, x, y) {
				queue = append(queue, int32(y*m.Width+x))
			}
		}
	}

	for i := 0; i < len(queue); i++ {
		cell := int(queue[i])
		x := cell % m.Width
		y := cell / m.Width
		nextDistance := int(distances[cell]) + 1
		if nextDistance > maxDistance {
			continue
		}
		nbs, count := neighborIndexes(x, y, m.Width, m.Height)
		for i := 0; i < count; i++ {
			nb := nbs[i]
			if isWater(m, nb%m.Width, nb/m.Width) || nextDistance >= int(distances[nb]) {
				continue
			}
			distances[nb] = uint16(nextDistance)
			queue = append(queue, int32(nb))
		}
	}

	return distances
}

func maxWaterDistance(m *Map) int {
	return min(m.Width+m.Height+1, 65535)
}

func waterDistanceAt(m *Map, distances []uint16, x, y int) int {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return maxWaterDistance(m)
	}
	return int(distances[y*m.Width+x])
}

func neighborIndexes(x, y, width, height int) ([4]int, int) {
	var result [4]int
	i := 0
	if x > 0 {
		result[i] = y*width + x - 1
		i++
	}
	if x < width-1 {
		result[i] = y*width + x + 1
		i++
	}
	if y > 0 {
		result[i] = (y-1)*width + x
		i++
	}
	if y < height-1 {
		result[i] = (y+1)*width + x
		i++
	}
	return result, i
}

func sortRiverCandidates(candidates []riverCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].height != candidates[j].height {
			return candidates[i].height > candidates[j].height
		}
		if candidates[i].y != candidates[j].y {
			return candidates[i].y < candidates[j].y
		}
		return candidates[i].x < candidates[j].x
	})
}

func traceRiver(m *Map, start riverCandidate, config Config, riverNoise *valueNoise, waterDistance []uint16, riverIndex int) [][2]int {
	path := [][2]int{{start.x, start.y}}
	visited := map[[2]int]struct{}{{start.x, start.y}: {}}
	current := [2]int{start.x, start.y}
	var previous *[2]int

	for step := 0; step < max(config.Width, config.Height); step++ {
		nbs := neighbors8(current[0], current[1], config.Width, config.Height)
		for _, nb := range nbs {
			if isWater(m, nb[0], nb[1]) {
				path = append(path, nb)
				return path
			}
		}

		next, ok := bestRiverStep(m, nbs, visited, current, previous, config, riverNoise, waterDistance, riverIndex)
		if !ok {
			break
		}
		path = append(path, next)
		visited[next] = struct{}{}
		prev := current
		previous = &prev
		current = next
		if isRiver(m, current[0], current[1]) && len(path) > 4 {
			return path
		}
	}

	return path
}

func bestRiverStep(m *Map, nbs [][2]int, visited map[[2]int]struct{}, current [2]int, previous *[2]int, config Config, riverNoise *valueNoise, waterDistance []uint16, riverIndex int) ([2]int, bool) {
	var best [2]int
	bestScore := math.Inf(1)
	found := false
	for _, nb := range nbs {
		if _, ok := visited[nb]; ok {
			continue
		}
		score := scoreRiverStep(m, nb, current, previous, config, riverNoise, waterDistance, riverIndex)
		if score < bestScore {
			best = nb
			bestScore = score
			found = true
		}
	}
	return best, found
}

func scoreRiverStep(m *Map, cell, current [2]int, previous *[2]int, config Config, riverNoise *valueNoise, waterDistance []uint16, riverIndex int) float64 {
	height := m.heightAt(cell[0], cell[1])
	currentHeight := m.heightAt(current[0], current[1])
	edgeDistance := min(min(cell[0], cell[1]), min(config.Width-1-cell[0], config.Height-1-cell[1]))
	downhill := height - currentHeight
	maxDistance := float64(max(config.Width, config.Height))
	cellWaterDistance := float64(waterDistanceAt(m, waterDistance, cell[0], cell[1]))
	currentWaterDistance := float64(waterDistanceAt(m, waterDistance, current[0], current[1]))
	waterDirection := (cellWaterDistance - currentWaterDistance) / maxDistance
	meander := riverNoise.octaveNoise2D(float64(cell[0])*config.RiverMeanderScale+float64(riverIndex)*19, float64(cell[1])*config.RiverMeanderScale+float64(riverIndex)*31, 3, 0.58) - 0.5
	turnPenalty := 0.0
	if previous != nil {
		turnPenalty = riverTurnPenalty(cell, current, *previous)
	}

	return height +
		math.Max(0, downhill)*1.35 +
		cellWaterDistance/maxDistance*0.22 +
		math.Max(0, waterDirection)*0.35 -
		math.Max(0, -waterDirection)*0.08 +
		float64(edgeDistance)/float64(max(config.Width, config.Height))*0.1 -
		meander*config.RiverMeanderStrength +
		turnPenalty
}

func riverTurnPenalty(cell, current, previous [2]int) float64 {
	previousDx := current[0] - previous[0]
	previousDy := current[1] - previous[1]
	nextDx := cell[0] - current[0]
	nextDy := cell[1] - current[1]
	dot := previousDx*nextDx + previousDy*nextDy
	if dot < 0 {
		return 0.45
	}
	if dot == 0 {
		return -0.03
	}
	return 0.04
}

func paintRiverPath(m *Map, path [][2]int, config Config, riverNoise *valueNoise, riverIndex int) {
	landPath := make([][2]int, 0, len(path))
	for _, cell := range path {
		if !isWater(m, cell[0], cell[1]) {
			landPath = append(landPath, cell)
		}
	}
	maxWidth := max(config.RiverMinWidth, config.RiverMaxWidth)
	for index, cell := range landPath {
		flow := 1.0
		if len(landPath) > 1 {
			flow = float64(index) / float64(len(landPath)-1)
		}
		widthNoise := riverNoise.octaveNoise2D(float64(cell[0])*config.RiverMeanderScale*0.75+float64(riverIndex)*41, float64(cell[1])*config.RiverMeanderScale*0.75+float64(riverIndex)*43, 2, 0.5) - 0.5
		width := clamp(float64(config.RiverMinWidth)+flow*float64(maxWidth-config.RiverMinWidth)+widthNoise*1.15, float64(config.RiverMinWidth), float64(maxWidth))
		paintRiverChannel(m, cell, width, flow)
	}
}

func paintRiverChannel(m *Map, center [2]int, width, flow float64) {
	radius := max(0, int(math.Ceil(width/2))-1)
	for y := center[1] - radius; y <= center[1]+radius; y++ {
		for x := center[0] - radius; x <= center[0]+radius; x++ {
			if x < 0 || y < 0 || x >= m.Width || y >= m.Height || isWater(m, x, y) {
				continue
			}
			distance := math.Hypot(float64(x-center[0]), float64(y-center[1]))
			if distance <= width/2 {
				markRiverCell(m, x, y, riverDepth(width, flow, distance))
			}
		}
	}
	markRiverCell(m, center[0], center[1], riverDepth(width, flow, 0))
}

func riverDepth(width, flow, distanceFromCenter float64) uint8 {
	if width <= 1 {
		return uint8(clamp(math.Round(1+flow*2), 1, 7))
	}
	halfWidth := math.Max(width/2, 0.5)
	centerBias := clamp(1-distanceFromCenter/halfWidth, 0, 1)
	depth := 1 + centerBias*4 + flow*2
	return uint8(clamp(math.Round(depth), 1, 7))
}

func markRiverCell(m *Map, x, y int, depth uint8) {
	ch, idx := chunkCell(m, x, y)
	base := ch.BaseCell(idx)
	ch.SetBase(idx, world.PackBase(world.BiomeRiverValley, world.SoilSilt, base.Elevation(), base.Flags()))
	ch.SetWater(idx, world.PackWater(world.WaterRiver, depth, false))
	ch.SetCover(idx, world.PackCover(world.CoverNone, 0, 0))
	ch.SetStock(idx, 0)
}

func setCell(m *Map, x, y int, base world.BaseCell, water world.WaterCell, cover world.CoverCell, stock uint16) {
	ch, idx := chunkCell(m, x, y)
	ch.SetBase(idx, base)
	ch.SetWater(idx, water)
	ch.SetCover(idx, cover)
	ch.SetStock(idx, stock)
	ch.Meta[idx] = uint8(clamp(math.Round(m.heightAt(x, y)*255), 0, 255))
}

func setTemperature(m *Map, x, y int, temperature float64) {
	ch, idx := chunkCell(m, x, y)
	ch.Temperature[idx] = uint8(clamp(math.Round(temperature*255), 0, 255))
}

func classifyWaterBodies(m *Map, config Config) {
	visited := make([]bool, config.Width*config.Height)
	for y := 0; y < config.Height; y++ {
		for x := 0; x < config.Width; x++ {
			index := y*config.Width + x
			if visited[index] || !isWater(m, x, y) {
				continue
			}
			body, touchesEdge := collectWaterBody(m, index, visited)
			if touchesEdge {
				continue
			}
			for _, cell := range body {
				index := int(cell)
				ch, idx := chunkCell(m, index%m.Width, index/m.Width)
				ch.SetWater(idx, world.PackWater(world.WaterLake, 4, false))
			}
		}
	}
}

func collectWaterBody(m *Map, start int, visited []bool) ([]int32, bool) {
	queue := []int32{int32(start)}
	body := make([]int32, 0)
	touchesEdge := false
	visited[start] = true

	for i := 0; i < len(queue); i++ {
		cell := int(queue[i])
		x := cell % m.Width
		y := cell / m.Width
		if x == 0 || y == 0 || x == m.Width-1 || y == m.Height-1 {
			touchesEdge = true
			body = nil
		}
		if !touchesEdge {
			body = append(body, int32(cell))
		}
		nbs, count := neighborIndexes(x, y, m.Width, m.Height)
		for j := 0; j < count; j++ {
			nb := nbs[j]
			if visited[nb] || !isWater(m, nb%m.Width, nb/m.Width) {
				continue
			}
			visited[nb] = true
			queue = append(queue, int32(nb))
		}
	}

	return body, touchesEdge
}

func chunkCell(m *Map, x, y int) (*world.Chunk, uint16) {
	coord, idx := world.ToChunkCoord(int32(x), int32(y))
	ch := m.Chunks[coord]
	if ch == nil {
		ch = world.NewChunk(coord.X, coord.Y)
		m.Chunks[coord] = ch
	}
	return ch, idx
}

func isWater(m *Map, x, y int) bool {
	ch, idx := chunkCell(m, x, y)
	return ch.WaterCell(idx).Kind() != world.WaterNone
}

func isRiver(m *Map, x, y int) bool {
	ch, idx := chunkCell(m, x, y)
	return ch.WaterCell(idx).Kind() == world.WaterRiver
}

func hasLandNeighbor(m *Map, x, y int) bool {
	for _, nb := range neighbors(x, y, m.Width, m.Height) {
		if !isWater(m, nb[0], nb[1]) {
			return true
		}
	}
	return false
}

func hasWaterNeighbor(m *Map, x, y int) bool {
	for _, nb := range neighbors(x, y, m.Width, m.Height) {
		if isWater(m, nb[0], nb[1]) {
			return true
		}
	}
	return false
}

func markShallow(m *Map, x, y, distance int) {
	ch, idx := chunkCell(m, x, y)
	kind := ch.WaterCell(idx).Kind()
	ch.SetWater(idx, world.PackWater(kind, uint8(clamp(float64(distance), 1, 7)), true))
}

func collectStats(m *Map) Stats {
	var stats Stats
	for y := 0; y < m.Height; y++ {
		for x := 0; x < m.Width; x++ {
			ch, idx := chunkCell(m, x, y)
			water := ch.WaterCell(idx)
			cover := ch.CoverCell(idx)
			base := ch.BaseCell(idx)
			stock := int(ch.Stock[idx])
			if water.Kind() == world.WaterNone {
				stats.Land++
			} else {
				stats.Water++
				if water.Kind() == world.WaterRiver {
					stats.River++
				}
				if water.Tidal() {
					stats.Shallow++
				}
			}
			switch cover.Kind() {
			case world.CoverBirchForest, world.CoverPineForest, world.CoverMixedForest, world.CoverDryBush:
				stats.Forest++
				stats.WoodStock += stock
				if cover.Kind() == world.CoverDryBush {
					stats.DryBush++
				}
			}
			if base.Soil() == world.SoilRocky {
				stats.Rock++
				stats.StoneStock += stock
			}
			if base.Biome() == world.BiomeMountain {
				stats.Mountain++
			}
		}
	}
	return stats
}

func clearDirty(m *Map) {
	for _, ch := range m.Chunks {
		ch.Dirty = false
	}
}

func neighbors(x, y, width, height int) [][2]int {
	result := make([][2]int, 0, 4)
	if x > 0 {
		result = append(result, [2]int{x - 1, y})
	}
	if x < width-1 {
		result = append(result, [2]int{x + 1, y})
	}
	if y > 0 {
		result = append(result, [2]int{x, y - 1})
	}
	if y < height-1 {
		result = append(result, [2]int{x, y + 1})
	}
	return result
}

func neighbors8(x, y, width, height int) [][2]int {
	result := neighbors(x, y, width, height)
	for _, offset := range [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		nx := x + offset[0]
		ny := y + offset[1]
		if nx >= 0 && ny >= 0 && nx < width && ny < height {
			result = append(result, [2]int{nx, ny})
		}
	}
	return result
}

func nearbyRiver(m *Map, x, y, radius int) bool {
	for yy := y - radius; yy <= y+radius; yy++ {
		for xx := x - radius; xx <= x+radius; xx++ {
			if xx >= 0 && yy >= 0 && xx < m.Width && yy < m.Height && isRiver(m, xx, yy) {
				return true
			}
		}
	}
	return false
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
