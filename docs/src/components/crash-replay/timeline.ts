/**
 * Keyframe timelines for SVG animation. Pure: the caller owns the clock, so the
 * server-rendered poster, the animation loop and reduced motion all sample the
 * same tracks.
 */

type Ease = "linear" | "out" | "hold";

export interface Keyframe {
  /** Time on the loop clock, in ms. Keyframes must be sorted ascending. */
  t: number;
  /** Position in SVG user units. Omitted: carried forward from the previous keyframe. */
  x?: number;
  y?: number;
  /** 0 to 1. Omitted: carried forward. */
  opacity?: number;
  /** Omitted: carried forward. */
  scale?: number;
  /** Step function: applies from this keyframe on. Omitted: carried forward. */
  state?: string;
  /**
   * Easing of the segment that arrives at this keyframe. "hold" keeps the
   * previous keyframe's values until this time, so it works as a dwell.
   */
  ease?: Ease;
}

interface Resolved {
  readonly t: number;
  readonly x: number;
  readonly y: number;
  readonly opacity: number;
  readonly scale: number;
  readonly state: string;
  readonly ease: Ease;
}

export interface Track {
  readonly id: string;
  readonly keys: readonly Resolved[];
}

interface Pose {
  /** False outside the track's time range. */
  readonly alive: boolean;
  readonly x: number;
  readonly y: number;
  readonly opacity: number;
  readonly scale: number;
  readonly state: string;
}

const EASE: Readonly<Record<Exclude<Ease, "hold">, (p: number) => number>> = {
  linear: (p) => p,
  out: (p) => 1 - (1 - p) ** 3,
};

const DEAD: Pose = { alive: false, x: 0, y: 0, opacity: 0, scale: 1, state: "" };

/** Builds a track, carrying omitted values forward. Throws if keyframes are unsorted. */
export function track(id: string, keys: readonly Keyframe[]): Track {
  let x = 0;
  let y = 0;
  let opacity = 1;
  let scale = 1;
  let state = "";
  const resolved = keys.map((k, i): Resolved => {
    if (i > 0 && k.t < keys[i - 1].t) {
      throw new Error(`Track "${id}": keyframe ${i} (t=${k.t}) is before the previous one`);
    }
    x = k.x ?? x;
    y = k.y ?? y;
    opacity = k.opacity ?? opacity;
    scale = k.scale ?? scale;
    state = k.state ?? state;
    return { t: k.t, x, y, opacity, scale, state, ease: k.ease ?? "linear" };
  });
  return { id, keys: resolved };
}

/** Evaluates a track at time `t` (ms). */
function sample(tr: Track, t: number): Pose {
  const ks = tr.keys;
  const first = ks[0];
  const last = ks[ks.length - 1];
  if (t < first.t || t > last.t) return DEAD;

  let i = 0;
  while (i < ks.length - 1 && ks[i + 1].t <= t) i++;
  const a = ks[i];
  const b = ks[i + 1];
  const base: Pose = { alive: true, x: a.x, y: a.y, opacity: a.opacity, scale: a.scale, state: a.state };
  if (!b || b.ease === "hold") return base;

  const p = (t - a.t) / (b.t - a.t);
  const e = EASE[b.ease as Exclude<Ease, "hold">](p);
  return {
    alive: true,
    x: a.x + (b.x - a.x) * e,
    y: a.y + (b.y - a.y) * e,
    opacity: a.opacity + (b.opacity - a.opacity) * e,
    scale: a.scale + (b.scale - a.scale) * e,
    state: a.state,
  };
}

const fmt = (n: number): string => String(Math.round(n * 100) / 100);

/** The attributes a tracked SVG element carries at time `t`. */
export function attrsAt(tr: Track, t: number): { transform: string; opacity: string; state: string } {
  const p = sample(tr, t);
  const transform = `translate(${fmt(p.x)} ${fmt(p.y)})` + (p.scale === 1 ? "" : ` scale(${fmt(p.scale)})`);
  return { transform, opacity: fmt(p.alive ? p.opacity : 0), state: p.state };
}
