/**
 * crash-replay.client.ts — Plays the crash replay diagram (CrashReplay.astro).
 *
 * One requestAnimationFrame loop writes each tracked element's pose. The loop
 * runs only while the diagram is on screen, the tab is visible and the reader
 * has not paused it. With reduced motion the loop stops and the poster frame
 * shows; the setting is followed live. mount() calls the teardown on
 * astro:before-swap, so the loop, the observer and the listeners all stop when
 * the reader navigates away.
 */

import { mount } from "@cloudflare/nimbus-docs/client";
import { DURATION, POSTER_T, SCENES } from "./scene";
import { attrsAt, type Track } from "./timeline";

interface Part {
  readonly el: SVGElement;
  readonly tr: Track;
  /** The last written value, so only changes touch the DOM. */
  last: string;
}

function initCrashReplay(root: HTMLElement): () => void {
  const progress = root.querySelector<HTMLElement>("[data-crash-progress]");
  const toggle = root.querySelector<HTMLButtonElement>("[data-crash-toggle]");
  const toggleLabel = root.querySelector<HTMLElement>("[data-crash-toggle-label]");
  const restart = root.querySelector<HTMLButtonElement>("[data-crash-restart]");
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)");

  // Tracked elements of both compositions.
  const parts: Part[] = [];
  for (const svg of root.querySelectorAll<SVGSVGElement>("svg[data-layout]")) {
    const scene = SCENES[svg.dataset.layout as keyof typeof SCENES];
    for (const el of svg.querySelectorAll<SVGElement>("[data-t]")) {
      const tr: Track | undefined = scene[el.dataset.t ?? ""];
      if (tr) parts.push({ el, tr, last: "" });
    }
  }

  let time = 0;
  let last = 0;
  let raf = 0;
  let inView = false;
  let paused = false;

  function render(t: number): void {
    for (const p of parts) {
      const a = attrsAt(p.tr, t);
      const key = `${a.transform}|${a.opacity}|${a.state}`;
      if (key === p.last) continue;
      p.last = key;
      p.el.setAttribute("transform", a.transform);
      p.el.setAttribute("opacity", a.opacity);
      p.el.setAttribute("data-state", a.state);
    }
    progress?.style.setProperty("--p", String(t / DURATION));
  }

  function tick(now: number): void {
    // Clamp long frames so a stall cannot skip a beat, and never run the clock backwards.
    time = (time + Math.max(0, Math.min(now - last, 64))) % DURATION;
    last = now;
    render(time);
    raf = requestAnimationFrame(tick);
  }

  function sync(): void {
    const shouldRun = inView && !document.hidden && !paused && !reduce.matches;
    if (shouldRun && !raf) {
      last = performance.now();
      raf = requestAnimationFrame(tick);
    } else if (!shouldRun && raf) {
      cancelAnimationFrame(raf);
      raf = 0;
    }
  }

  const observer = new IntersectionObserver(
    ([entry]) => {
      inView = entry.isIntersecting;
      sync();
    },
    { threshold: 0.3 },
  );
  observer.observe(root);

  const onVisibility = (): void => sync();
  const onReduceChange = (): void => {
    render(reduce.matches ? POSTER_T : time);
    sync();
  };
  const onToggle = (): void => {
    paused = !paused;
    if (toggleLabel) toggleLabel.textContent = paused ? "play" : "pause";
    toggle?.toggleAttribute("data-paused", paused);
    toggle?.setAttribute("aria-label", paused ? "Play" : "Pause");
    sync();
  };
  const onRestart = (): void => {
    time = 0;
    last = performance.now();
    render(time);
  };

  document.addEventListener("visibilitychange", onVisibility);
  reduce.addEventListener("change", onReduceChange);
  toggle?.addEventListener("click", onToggle);
  restart?.addEventListener("click", onRestart);

  // Shows the controls. Without this attribute they stay hidden, so a page without JavaScript shows only the poster.
  root.setAttribute("data-crash-ready", "");

  // Start from the first frame, out of view, so the scene fades in when it scrolls in.
  if (!reduce.matches) render(time);
  sync();

  return () => {
    if (raf) cancelAnimationFrame(raf);
    raf = 0;
    observer.disconnect();
    document.removeEventListener("visibilitychange", onVisibility);
    reduce.removeEventListener("change", onReduceChange);
    toggle?.removeEventListener("click", onToggle);
    restart?.removeEventListener("click", onRestart);
  };
}

mount("[data-crash]", initCrashReplay);
