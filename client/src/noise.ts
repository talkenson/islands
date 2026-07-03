export interface ColorNoise {
  noise2D(x: number, y: number): number;
  octaveNoise2D(x: number, y: number, octaves: number, persistence: number): number;
}

export class ValueNoise implements ColorNoise {
  private readonly values = new Map<string, number>();

  constructor(private readonly seed: string) {}

  noise2D(x: number, y: number): number {
    const x0 = Math.floor(x);
    const y0 = Math.floor(y);
    const x1 = x0 + 1;
    const y1 = y0 + 1;
    const sx = fade(x - x0);
    const sy = fade(y - y0);

    const n0 = lerp(this.lattice(x0, y0), this.lattice(x1, y0), sx);
    const n1 = lerp(this.lattice(x0, y1), this.lattice(x1, y1), sx);
    return lerp(n0, n1, sy);
  }

  octaveNoise2D(x: number, y: number, octaves: number, persistence: number): number {
    let amplitude = 1;
    let frequency = 1;
    let total = 0;
    let maxValue = 0;

    for (let i = 0; i < octaves; i++) {
      total += this.noise2D(x * frequency, y * frequency) * amplitude;
      maxValue += amplitude;
      amplitude *= persistence;
      frequency *= 2;
    }

    return total / maxValue;
  }

  private lattice(ix: number, iy: number): number {
    const cacheKey = `${ix},${iy}`;
    const cached = this.values.get(cacheKey);
    if (cached !== undefined) {
      return cached;
    }
    const value = new Random(makeSeed(this.seed, cacheKey)).next();
    this.values.set(cacheKey, value);
    return value;
  }
}

export function makeSeed(seed: string, salt: string): string {
  return `${seed}:${salt}`;
}

class Random {
  private state: number;

  constructor(seed: string) {
    this.state = hashSeed(seed)();
  }

  next(): number {
    this.state = (this.state + 0x6d2b79f5) >>> 0;
    let t = this.state;
    t = Math.imul((t ^ (t >>> 15)) >>> 0, (t | 1) >>> 0) >>> 0;
    t = (t ^ (t + Math.imul((t ^ (t >>> 7)) >>> 0, (t | 61) >>> 0))) >>> 0;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }
}

function hashSeed(seed: string): () => number {
  let h = (1779033703 ^ [...seed].length) >>> 0;

  for (const char of seed) {
    h = Math.imul((h ^ (char.codePointAt(0) || 0)) >>> 0, 3432918353) >>> 0;
    h = rotateLeft32(h, 13);
  }

  return () => {
    h = Math.imul((h ^ (h >>> 16)) >>> 0, 2246822507) >>> 0;
    h = Math.imul((h ^ (h >>> 13)) >>> 0, 3266489909) >>> 0;
    h = (h ^ (h >>> 16)) >>> 0;
    return h;
  };
}

function rotateLeft32(value: number, shift: number): number {
  return ((value << shift) | (value >>> (32 - shift))) >>> 0;
}

function fade(t: number): number {
  return t * t * t * (t * (t * 6 - 15) + 10);
}

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}
