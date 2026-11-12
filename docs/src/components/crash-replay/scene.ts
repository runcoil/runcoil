/**
 * Scene of the crash replay diagram: one Run crashes on worker-a and replays on
 * worker-b. Holds the geometry of the two compositions (wide and stacked) and
 * the 15 s loop timeline both share. Pure data, so the poster and the animation
 * always agree.
 */
import { track, type Keyframe, type Track } from "./timeline";

export const DURATION = 15000;
/** Poster frame: attempt 2 replays Steps 1 and 2 on worker-b. */
export const POSTER_T = 10600;

interface Beat {
  /** Loop time (ms) the beat starts. */
  readonly at: number;
  readonly text: string;
}

/** One line per beat, shown in the subtitle strip and read by screen readers. */
export const BEATS: readonly Beat[] = [
  { at: 700, text: "Attempt 1 on worker-a" },
  { at: 5350, text: "Steps 1–2 saved" },
  { at: 6100, text: "worker-a lost during Step 3" },
  { at: 7400, text: "Attempt 2 on worker-b" },
  { at: 9200, text: "Steps 1–2 return stored results" },
  { at: 10700, text: "Steps 3–4 run live, then the Run completes" },
];
const BEAT_END = 13600;

interface WorkerLayout {
  readonly box: { readonly x: number; readonly y: number; readonly w: number; readonly h: number };
  readonly titleY: number;
  readonly labelY: number;
  readonly cellY: number;
  readonly cellH: number;
  readonly cellW: number;
  readonly cellXs: readonly number[];
}

export interface Layout {
  readonly key: "wide" | "narrow";
  readonly w: number;
  readonly h: number;
  /** Label and subtitle sizes, in user units. */
  readonly labelSize: number;
  readonly subtitleSize: number;
  readonly server: { readonly x: number; readonly y: number; readonly w: number; readonly h: number };
  readonly serverTitleY: number;
  readonly savedY: number;
  readonly rows: {
    readonly x: number;
    readonly w: number;
    readonly h: number;
    readonly ys: readonly number[];
    readonly chipX: number;
    readonly textX: number;
  };
  readonly dividerY: number;
  readonly queueLabelY: number;
  readonly queueY: number;
  readonly queueLane: { readonly x1: number; readonly x2: number };
  /** Where a Run token starts, inside the queue. */
  readonly queueSpawn: { readonly x: number; readonly y: number };
  /** x of the vertical dispatch line that feeds both workers. */
  readonly busX: number;
  /** Dotted connector drawn between the queue and the worker lanes. */
  readonly bus: string;
  readonly workers: { readonly a: WorkerLayout; readonly b: WorkerLayout };
  readonly subtitleY: number;
  /** Inner padding of the boxes. */
  readonly pad: number;
}

/** Wide: server left, workers right. Used when the diagram column is 600px or wider. */
const WIDE: Layout = {
  key: "wide",
  w: 960,
  h: 440,
  // Sized for the docs column: the 960-unit canvas renders at about 0.7 scale.
  labelSize: 18,
  subtitleSize: 22,
  server: { x: 40, y: 36, w: 320, h: 340 },
  serverTitleY: 64,
  savedY: 94,
  rows: { x: 64, w: 272, h: 36, ys: [130, 174, 218, 262], chipX: 84, textX: 104 },
  dividerY: 304,
  queueLabelY: 332,
  queueY: 352,
  queueLane: { x1: 64, x2: 336 },
  queueSpawn: { x: 84, y: 352 },
  busX: 430,
  bus: "M360 352 H430 V130 H500 M430 304 H500",
  workers: {
    a: {
      box: { x: 500, y: 36, w: 420, h: 150 },
      titleY: 64,
      labelY: 92,
      cellY: 100,
      cellH: 60,
      cellW: 84,
      cellXs: [524, 620, 716, 812],
    },
    b: {
      box: { x: 500, y: 210, w: 420, h: 150 },
      titleY: 238,
      labelY: 266,
      cellY: 274,
      cellH: 60,
      cellW: 84,
      cellXs: [524, 620, 716, 812],
    },
  },
  subtitleY: 414,
  pad: 24,
};

/** Narrow: server on top, workers below, dispatch line down the left edge. Used below 600px. */
const NARROW: Layout = {
  key: "narrow",
  w: 360,
  h: 700,
  labelSize: 13,
  subtitleSize: 15,
  server: { x: 64, y: 12, w: 284, h: 280 },
  serverTitleY: 40,
  savedY: 62,
  rows: { x: 84, w: 252, h: 30, ys: [87, 123, 159, 195], chipX: 96, textX: 110 },
  dividerY: 228,
  queueLabelY: 250,
  queueY: 272,
  queueLane: { x1: 84, x2: 330 },
  queueSpawn: { x: 96, y: 272 },
  busX: 36,
  bus: "M64 272 H36 V582 H64 M36 412 H64",
  workers: {
    a: {
      box: { x: 64, y: 324, w: 284, h: 150 },
      titleY: 352,
      labelY: 386,
      cellY: 392,
      cellH: 40,
      cellW: 56,
      cellXs: [84, 148, 212, 276],
    },
    b: {
      box: { x: 64, y: 494, w: 284, h: 150 },
      titleY: 522,
      labelY: 556,
      cellY: 562,
      cellH: 40,
      cellW: 56,
      cellXs: [84, 148, 212, 276],
    },
  },
  subtitleY: 680,
  pad: 20,
};

interface Point {
  readonly x: number;
  readonly y: number;
}

const cellCenters = (w: WorkerLayout): Point[] =>
  w.cellXs.map((x) => ({ x: x + w.cellW / 2, y: w.cellY + w.cellH / 2 }));

/** Keyframes that hold a state from each (time, state) pair to the loop end. */
const states = (id: string, initial: string, changes: readonly (readonly [number, string])[]): Track =>
  track(id, [
    { t: 0, state: initial },
    ...changes.map(([t, state]): Keyframe => ({ t, state, ease: "hold" })),
    { t: DURATION, ease: "hold" },
  ]);

/**
 * A beat is shown from its start to the next beat. Lines do not overlap: the
 * old line fades out in 150 ms, then the new one fades in.
 */
const beatTrack = (i: number): Track => {
  const start = BEATS[i].at;
  const end = i + 1 < BEATS.length ? BEATS[i + 1].at : BEAT_END;
  return track(`beat-${i + 1}`, [
    { t: start, opacity: 0 },
    { t: start + 150, ease: "hold" },
    { t: start + 350, opacity: 1 },
    { t: end, ease: "hold" },
    { t: end + 150, opacity: 0 },
    { t: DURATION, ease: "hold" },
  ]);
};

/**
 * Every tracked element of one composition, keyed by its data-t id.
 * Times are the loop clock in ms; positions come from the layout.
 */
function buildScene(L: Layout): Record<string, Track> {
  const q = L.queueSpawn;
  const ca = cellCenters(L.workers.a);
  const cb = cellCenters(L.workers.b);
  const rowY = L.rows.ys;
  const rowSlot = (i: number): Point => ({ x: L.rows.chipX, y: rowY[i] });

  // Attempt 1 on worker-a: Steps 1–2 saved, Step 3 lost when the worker dies.
  const tokenA = track("token-a", [
    { t: 700, x: q.x, y: q.y, opacity: 0, state: "queued" },
    { t: 1000, opacity: 1 },
    { t: 1300, ease: "hold" },
    { t: 1800, x: L.busX },
    { t: 2200, y: ca[0].y },
    { t: 2500, x: ca[0].x, state: "active" },
    { t: 3500, ease: "hold" },
    { t: 3800, x: ca[1].x, ease: "out" },
    { t: 4800, ease: "hold" },
    { t: 5100, x: ca[2].x, ease: "out" },
    { t: 6100, ease: "hold", state: "lost" },
    { t: 6600, opacity: 0 },
  ]);

  // Attempt 2 on worker-b: replays Steps 1–2, runs Steps 3–4 live.
  const tokenB = track("token-b", [
    { t: 7400, x: q.x, y: q.y, opacity: 0, state: "queued" },
    { t: 7700, opacity: 1 },
    { t: 8000, ease: "hold" },
    { t: 8500, x: L.busX },
    { t: 8900, y: cb[0].y },
    { t: 9200, x: cb[0].x, state: "active" },
    { t: 9700, ease: "hold" },
    { t: 9950, x: cb[1].x, ease: "out" },
    { t: 10450, ease: "hold" },
    { t: 10700, x: cb[2].x, ease: "out" },
    { t: 11700, ease: "hold" },
    { t: 11950, x: cb[3].x, ease: "out" },
    { t: 12950, ease: "hold", state: "done" },
    { t: 13450, scale: 1.8, opacity: 0, ease: "out" },
  ]);

  // A checkpoint chip leaves a finished Step and lands in its saved row.
  const chip = (n: number, done: number, from: Point, row: number): Track =>
    track(`chip-${n}`, [
      { t: done, x: from.x, y: from.y, opacity: 0 },
      { t: done + 50, opacity: 1 },
      { t: done + 550, ...rowSlot(row), ease: "out" },
      { t: DURATION, ease: "hold" },
    ]);

  // A stored result leaves its saved row and returns to the replayed Step.
  const ret = (n: number, start: number, row: number, to: Point): Track =>
    track(`ret-${n}`, [
      { t: start, ...rowSlot(row), opacity: 0 },
      { t: start + 50, opacity: 1 },
      { t: start + 500, x: to.x, y: to.y, ease: "out" },
      { t: start + 550, opacity: 0 },
      { t: DURATION, ease: "hold" },
    ]);

  const attemptLabel = (id: string, on: number): Track =>
    track(id, [
      { t: 0, opacity: 0 },
      { t: on, ease: "hold" },
      { t: on + 200, opacity: 1 },
      { t: DURATION, ease: "hold" },
    ]);

  const scene: Record<string, Track> = {
    scene: track("scene", [
      { t: 0, opacity: 0 },
      { t: 400, opacity: 1 },
      { t: BEAT_END, ease: "hold" },
      { t: 14000, opacity: 0 },
      { t: DURATION, ease: "hold" },
    ]),
    "token-a": tokenA,
    "token-b": tokenB,
    "chip-1": chip(1, 3500, ca[0], 0),
    "chip-2": chip(2, 4800, ca[1], 1),
    "chip-3": chip(3, 11700, cb[2], 2),
    "chip-4": chip(4, 12950, cb[3], 3),
    "ret-1": ret(1, 9200, 0, cb[0]),
    "ret-2": ret(2, 9950, 1, cb[1]),
    "row-1": states("row-1", "empty", [
      [4050, "saved"],
      [9200, "read"],
      [9700, "saved"],
    ]),
    "row-2": states("row-2", "empty", [
      [5350, "saved"],
      [9950, "read"],
      [10450, "saved"],
    ]),
    "row-3": states("row-3", "empty", [[12250, "saved"]]),
    "row-4": states("row-4", "empty", [[13500, "saved"]]),
    "box-a": states("box-a", "live", [
      [6100, "flicker"],
      [6500, "down"],
    ]),
    "att-a": attemptLabel("att-a", 700),
    "att-b": attemptLabel("att-b", 7400),
    "cell-a1": states("cell-a1", "idle", [
      [2500, "running"],
      [3500, "done"],
    ]),
    "cell-a2": states("cell-a2", "idle", [
      [3800, "running"],
      [4800, "done"],
    ]),
    "cell-a3": states("cell-a3", "idle", [
      [5100, "running"],
      [6100, "lost"],
    ]),
    "cell-a4": states("cell-a4", "idle", []),
    "cell-b1": states("cell-b1", "idle", [[9700, "replay"]]),
    "cell-b2": states("cell-b2", "idle", [[10450, "replay"]]),
    "cell-b3": states("cell-b3", "idle", [
      [10700, "running"],
      [11700, "done"],
    ]),
    "cell-b4": states("cell-b4", "idle", [
      [11950, "running"],
      [12950, "done"],
    ]),
  };
  BEATS.forEach((_, i) => {
    scene[`beat-${i + 1}`] = beatTrack(i);
  });
  return scene;
}

/** The tracks for each composition, built once at module load. */
export const SCENES: Readonly<Record<Layout["key"], Readonly<Record<string, Track>>>> = {
  wide: buildScene(WIDE),
  narrow: buildScene(NARROW),
};

/** The layouts, in the order the diagram stacks them. */
export const LAYOUTS: readonly Layout[] = [WIDE, NARROW];
