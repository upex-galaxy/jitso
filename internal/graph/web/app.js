// The jitso graph page: the sessions as stars, who launched whom as dashed
// lines and who talked to whom as glowing synapses. It polls api/snapshot.
// Every text that comes from a session (a title, a name, a path) reaches the
// page through textContent or the canvas, never as markup.
(() => {
  "use strict";

  const POLL_MS = 3000;
  const DIM = "#98a3cc";
  const FOLDER = "#a9c4ff";
  const FONT = 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';
  const COLOR = {
    live: "#cfe6ff", lead: "#ffe29a", unlaunched: "#8390c4", gone: "#ff857d",
    unknown: "#ffbd5c", closed: "#67709a", oneway: "#5ee7ff", twoway: "#ff7ad9",
  };

  const token = document.querySelector('meta[name="jitso-token"]').content;
  const $ = (id) => document.getElementById(id);
  const canvas = $("sky");
  const ctx = canvas.getContext("2d");
  const still = window.matchMedia("(prefers-reduced-motion: reduce)");

  // One layout per scope, so switching back finds every star where it was.
  const newLayout = () => ({ points: new Map(), shape: "", alpha: 0, view: { x: 0, y: 0, k: 1 }, moved: false });
  const layouts = { project: newLayout(), all: newLayout() };

  let data = null;
  let byKey = new Map();
  let shown = { nodes: [], hierarchy: [], synapses: [] };
  let scope = "project";
  let selected = null;
  let hovered = null;
  let query = "";
  let lost = "";
  let flight = null;
  let labelBoxes = [];
  let detailsShape = "";
  let width = 0;
  let height = 0;

  const layout = () => layouts[scope];
  const clamp = (v, lo, hi) => Math.min(hi, Math.max(lo, v));

  // ---------- what a session looks like ----------

  const isLead = (n) => n.role === "conductor" || n.type === "conductor";
  const radius = (n) => (n.id === null ? 4.5 : isLead(n) ? 10 : 7);
  const colorOf = (n) => (n.state === "live" ? (isLead(n) ? COLOR.lead : COLOR.live) : COLOR[n.state] || COLOR.live);
  const cut = (s, max) => {
    const chars = Array.from(s);
    return chars.length > max ? chars.slice(0, max - 1).join("") + "…" : s;
  };

  const titleOf = (n) => (n.id !== null ? (n.name && !n.name.includes("[jitso:") ? n.name : n.build || n.name || n.id) : n.title || n.name || n.folder || n.handle || n.key);

  // The lines of a label: what the session is, then the folder it lives in and,
  // for a launch, its role and id. The last one may carry the state's color.
  function labelOf(n) {
    const main = cut(titleOf(n), 30);
    const more = [];
    const add = (text, color) => more.push({ text: cut(text, 34), color });
    if (n.id !== null) {
      if (n.folder) add(n.folder, FOLDER);
      // Only a launch into another repository carries repo.
      if (n.repo && n.repo !== n.folder) add(n.repo, FOLDER);
      const kind = [n.role, n.type].filter(Boolean).filter((v, i, a) => a.indexOf(v) === i).join("/");
      const parts = [kind, n.id.slice(-4), n.state === "live" ? "" : n.state].filter(Boolean);
      add(parts.join(" · "), n.state === "gone" || n.state === "unknown" ? COLOR[n.state] : DIM);
    } else if (n.folder && (n.title || n.name)) {
      add(n.folder, FOLDER);
    } else {
      add("unlaunched", DIM);
    }
    return { main, more };
  }

  const widths = new Map();
  function textWidth(text, font) {
    const key = font + "\n" + text;
    let w = widths.get(key);
    if (w === undefined) {
      ctx.save();
      ctx.font = font;
      w = ctx.measureText(text).width;
      ctx.restore();
      widths.set(key, w);
    }
    return w;
  }
  const MAIN_FONT = "600 12px " + FONT;
  const SUB_FONT = "10.5px " + FONT;
  const boxWidth = (n) => {
    const l = labelOf(n);
    return Math.max(34, textWidth(l.main, MAIN_FONT), ...l.more.map((m) => textWidth(m.text, SUB_FONT))) + 22;
  };
  // The room a star and its label take from top to bottom.
  const boxHeight = (n) => 45 + 13 * labelOf(n).more.length;

  // ---------- layout ----------

  // A small stable hash, so a session starts at the same place on every load.
  function hash(text) {
    let h = 2166136261;
    for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), 16777619);
    return (h >>> 0) / 4294967296;
  }

  const edgesOf = (graph) => [
    ...graph.hierarchy.map((e) => ({ a: e.parent, b: e.child, rest: 125, pull: 0.07 })),
    ...graph.synapses.map((e) => ({ a: e.from, b: e.to, rest: 200, pull: 0.012 })),
  ];

  function place(l, graph) {
    const edges = edgesOf(graph);
    const fresh = graph.nodes.filter((n) => !l.points.has(n.key));
    // Connected sessions first, so a child or a peer starts next to its other end.
    const degree = (n) => edges.filter((e) => e.a === n.key || e.b === n.key).length;
    fresh.sort((a, b) => degree(b) - degree(a));
    let index = l.points.size;
    for (const n of fresh) {
      const edge = edges.find((e) => (e.a === n.key && l.points.has(e.b)) || (e.b === n.key && l.points.has(e.a)));
      const angle = hash(n.key) * Math.PI * 2;
      let x;
      let y;
      if (edge) {
        const other = l.points.get(edge.a === n.key ? edge.b : edge.a);
        x = other.x + Math.cos(angle) * 110;
        y = other.y + Math.sin(angle) * 90;
      } else {
        const r = 75 * Math.sqrt(index + 0.5);
        x = Math.cos(index * 2.39996) * r * 1.4 + Math.cos(angle) * 6;
        y = Math.sin(index * 2.39996) * r * 0.85 + Math.sin(angle) * 6;
      }
      l.points.set(n.key, { x, y, vx: 0, vy: 0, pinned: false, fresh: true });
      index++;
    }
  }

  // One step of the force layout: stars push each other away, edges pull their
  // ends, a weak gravity keeps the sky together and labels never share a place.
  function step(l, graph, alpha, settled) {
    const items = graph.nodes.map((n) => ({ p: l.points.get(n.key), w: boxWidth(n), h: boxHeight(n) }));
    for (let i = 0; i < items.length; i++) {
      const a = items[i].p;
      for (let j = i + 1; j < items.length; j++) {
        const b = items[j].p;
        const dx = b.x - a.x || 0.1;
        const dy = b.y - a.y || 0.1;
        const d2 = Math.max(dx * dx + dy * dy, 400);
        if (d2 > 360000) continue;
        const f = (4200 * alpha) / d2;
        a.vx -= dx * f * 0.1; a.vy -= dy * f * 0.1;
        b.vx += dx * f * 0.1; b.vy += dy * f * 0.1;
      }
    }
    for (const e of edgesOf(graph)) {
      const a = l.points.get(e.a);
      const b = l.points.get(e.b);
      if (!a || !b) continue;
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const d = Math.hypot(dx, dy) || 1;
      const f = ((d - e.rest) / d) * e.pull * alpha;
      a.vx += dx * f; a.vy += dy * f;
      b.vx -= dx * f; b.vy -= dy * f;
    }
    // A star that sits on a synapse of two other stars moves aside, so a line
    // never seems to stop at a session it does not belong to.
    for (const e of graph.synapses) {
      const a = l.points.get(e.from);
      const b = l.points.get(e.to);
      const dx = b.x - a.x;
      const dy = b.y - a.y;
      const d2 = dx * dx + dy * dy || 1;
      for (const { p } of items) {
        if (p === a || p === b) continue;
        const t = ((p.x - a.x) * dx + (p.y - a.y) * dy) / d2;
        if (t <= 0.08 || t >= 0.92) continue;
        const side = ((p.x - a.x) * -dy + (p.y - a.y) * dx) / Math.sqrt(d2);
        const near = 70 - Math.abs(side);
        if (near <= 0) continue;
        const push = (near / Math.sqrt(d2)) * 0.09 * alpha * (side < 0 ? -1 : 1);
        p.vx -= dy * push;
        p.vy += dx * push;
      }
    }
    const wide = width >= height ? 1 : -1;
    for (const { p } of items) {
      p.vx -= p.x * (wide > 0 ? 0.006 : 0.016) * alpha;
      p.vy -= p.y * (wide > 0 ? 0.016 : 0.006) * alpha;
      p.vx *= 0.6; p.vy *= 0.6;
      if (p.pinned) { p.vx = 0; p.vy = 0; continue; }
      // On a later change the stars already placed move less than the new ones.
      const give = settled && !p.fresh ? 0.3 : 1;
      p.x += clamp(p.vx, -30, 30) * give;
      p.y += clamp(p.vy, -30, 30) * give;
    }
    for (let i = 0; i < items.length; i++) {
      const a = items[i];
      for (let j = i + 1; j < items.length; j++) {
        const b = items[j];
        const dx = b.p.x - a.p.x;
        const dy = b.p.y - a.p.y;
        const ox = (a.w + b.w) / 2 - Math.abs(dx);
        const oy = (a.h + b.h) / 2 - Math.abs(dy);
        if (ox <= 0 || oy <= 0) continue;
        const alongX = ox / ((a.w + b.w) / 2) < oy / ((a.h + b.h) / 2);
        const push = (alongX ? ox : oy) * 0.25 * ((alongX ? dx : dy) < 0 ? -1 : 1);
        if (!a.p.pinned) { if (alongX) a.p.x -= push; else a.p.y -= push; }
        if (!b.p.pinned) { if (alongX) b.p.x += push; else b.p.y += push; }
      }
    }
  }

  function area() {
    const bar = document.querySelector(".bar").getBoundingClientRect();
    const notices = $("notices").getBoundingClientRect();
    const legend = $("legend").getBoundingClientRect();
    const panel = $("details").getBoundingClientRect();
    const top = Math.max(bar.bottom, notices.height ? notices.bottom : 0) + 14;
    const bottom = (legend.height ? legend.top : height) - 8;
    const right = panel.width && width > 720 ? panel.left - 12 : width - 12;
    return { left: 12, top, width: Math.max(120, right - 12), height: Math.max(120, bottom - top) };
  }

  function fitView() {
    const l = layout();
    if (!shown.nodes.length) return { x: width / 2, y: height / 2, k: 1 };
    let x0 = Infinity; let y0 = Infinity; let x1 = -Infinity; let y1 = -Infinity;
    // Labels keep their screen size at any zoom, so their room is taken off the
    // free area and only the distances between the stars are scaled.
    let side = 0;
    for (const n of shown.nodes) {
      const p = l.points.get(n.key);
      side = Math.max(side, boxWidth(n) / 2);
      x0 = Math.min(x0, p.x); x1 = Math.max(x1, p.x);
      y0 = Math.min(y0, p.y); y1 = Math.max(y1, p.y);
    }
    const a = area();
    const above = 24;
    const below = 60;
    const k = clamp(Math.min(
      Math.max(40, a.width - side * 2) / Math.max(1, x1 - x0),
      Math.max(40, a.height - above - below) / Math.max(1, y1 - y0),
    ), 0.15, 1.5);
    return {
      k,
      x: a.left + a.width / 2 - ((x0 + x1) / 2) * k,
      y: a.top + a.height / 2 + (above - below) / 2 - ((y0 + y1) / 2) * k,
    };
  }

  function flyTo(view) {
    if (still.matches) { Object.assign(layout().view, view); flight = null; return; }
    flight = { from: { ...layout().view }, to: view, start: performance.now() };
  }

  // Brings the layout of the current scope up to date. Nothing moves unless the
  // set of stars or edges changed.
  function arrange() {
    const l = layout();
    const shape = JSON.stringify([
      shown.nodes.map((n) => n.key).sort(),
      shown.hierarchy.map((e) => e.parent + ">" + e.child).sort(),
      shown.synapses.map((e) => [e.from, e.to].sort().join("~")).sort(),
    ]);
    if (shape === l.shape || !shown.nodes.length) return;
    const first = l.shape === "";
    l.shape = shape;
    place(l, shown);
    if (first) {
      for (let i = 0, alpha = 1; i < 420; i++, alpha = Math.max(0.03, alpha * 0.988)) step(l, shown, alpha, false);
      for (const p of l.points.values()) { p.fresh = false; p.vx = 0; p.vy = 0; }
      l.alpha = 0;
      Object.assign(l.view, fitView());
    } else {
      l.alpha = 0.35;
    }
  }

  // ---------- drawing ----------

  const backdrop = document.createElement("canvas");
  let twinklers = [];
  const sprites = new Map();

  function paintBackdrop(dpr) {
    backdrop.width = canvas.width;
    backdrop.height = canvas.height;
    const g = backdrop.getContext("2d");
    g.setTransform(dpr, 0, 0, dpr, 0, 0);
    g.fillStyle = "#05081a";
    g.fillRect(0, 0, width, height);
    const cloud = (x, y, r, color) => {
      const fill = g.createRadialGradient(x, y, 0, x, y, r);
      fill.addColorStop(0, color);
      fill.addColorStop(1, "rgba(5, 8, 26, 0)");
      g.fillStyle = fill;
      g.fillRect(0, 0, width, height);
    };
    cloud(width * 0.22, height * 0.3, width * 0.5, "rgba(52, 48, 140, 0.30)");
    cloud(width * 0.8, height * 0.75, width * 0.45, "rgba(18, 90, 130, 0.24)");
    cloud(width * 0.65, height * 0.15, width * 0.3, "rgba(110, 36, 110, 0.14)");
    let seed = 7;
    const random = () => ((seed = (seed * 48271) % 2147483647) / 2147483647);
    const count = Math.round((width * height) / 2600);
    twinklers = [];
    for (let i = 0; i < count; i++) {
      const s = { x: random() * width, y: random() * height, r: random() * 0.9 + 0.2, a: random() * 0.45 + 0.08, phase: random() * 7 };
      g.globalAlpha = s.a;
      g.fillStyle = i % 9 === 0 ? "#b9c8ff" : "#ffffff";
      g.beginPath();
      g.arc(s.x, s.y, s.r, 0, Math.PI * 2);
      g.fill();
      if (i % 14 === 0) twinklers.push(s);
    }
    g.globalAlpha = 1;
  }

  // The glow of one color, drawn once and scaled for every star.
  function sprite(color) {
    let s = sprites.get(color);
    if (!s) {
      s = document.createElement("canvas");
      s.width = s.height = 96;
      const g = s.getContext("2d");
      const fill = g.createRadialGradient(48, 48, 0, 48, 48, 48);
      fill.addColorStop(0, color + "cc");
      fill.addColorStop(0.22, color + "55");
      fill.addColorStop(1, color + "00");
      g.fillStyle = fill;
      g.fillRect(0, 0, 96, 96);
      sprites.set(color, s);
    }
    return s;
  }

  const screen = (p) => ({ x: p.x * layout().view.k + layout().view.x, y: p.y * layout().view.k + layout().view.y });
  const world = (x, y) => ({ x: (x - layout().view.x) / layout().view.k, y: (y - layout().view.y) / layout().view.k });
  const scaled = (n) => radius(n) * clamp(layout().view.k, 0.65, 1.35);

  // The sessions to keep bright, or null for all of them.
  function focusSet() {
    if (query) return new Set(shown.nodes.filter((n) => matches(n, query)).map((n) => n.key));
    if (!selected || !shown.nodes.some((n) => n.key === selected)) return null;
    const keys = new Set([selected]);
    for (const e of shown.synapses) if (e.from === selected || e.to === selected) { keys.add(e.from); keys.add(e.to); }
    for (const e of shown.hierarchy) if (e.parent === selected || e.child === selected) { keys.add(e.parent); keys.add(e.child); }
    return keys;
  }

  const edgeAlpha = (a, b, focus) => {
    if (!focus) return 1;
    if (query) return focus.has(a) && focus.has(b) ? 1 : 0.15;
    return a === selected || b === selected ? 1 : 0.15;
  };

  function drawHierarchy(focus) {
    for (const e of shown.hierarchy) {
      const a = screen(layout().points.get(e.parent));
      const b = screen(layout().points.get(e.child));
      const d = Math.hypot(b.x - a.x, b.y - a.y);
      if (d < 20) continue;
      const ux = (b.x - a.x) / d;
      const uy = (b.y - a.y) / d;
      ctx.globalAlpha = edgeAlpha(e.parent, e.child, focus);
      ctx.strokeStyle = "rgba(150, 170, 232, 0.62)";
      ctx.lineWidth = 1.1;
      ctx.setLineDash([4, 5]);
      ctx.beginPath();
      ctx.moveTo(a.x + ux * 12, a.y + uy * 12);
      ctx.lineTo(b.x - ux * 12, b.y - uy * 12);
      ctx.stroke();
      ctx.setLineDash([]);
      // A small open chevron near the child says which way the launch went.
      const x = a.x + (b.x - a.x) * 0.66;
      const y = a.y + (b.y - a.y) * 0.66;
      ctx.beginPath();
      ctx.moveTo(x - ux * 6 - uy * 4, y - uy * 6 + ux * 4);
      ctx.lineTo(x, y);
      ctx.lineTo(x - ux * 6 + uy * 4, y - uy * 6 - ux * 4);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;
  }

  const along = (a, c, b, t) => ({
    x: (1 - t) * (1 - t) * a.x + 2 * (1 - t) * t * c.x + t * t * b.x,
    y: (1 - t) * (1 - t) * a.y + 2 * (1 - t) * t * c.y + t * t * b.y,
  });

  // The point at distance r from p toward q.
  const toward = (p, q, r) => {
    const d = Math.hypot(q.x - p.x, q.y - p.y) || 1;
    return { x: p.x + ((q.x - p.x) / d) * r, y: p.y + ((q.y - p.y) / d) * r };
  };

  function arrowHead(tip, from, color) {
    const angle = Math.atan2(tip.y - from.y, tip.x - from.x);
    ctx.fillStyle = color;
    ctx.beginPath();
    ctx.moveTo(tip.x, tip.y);
    ctx.lineTo(tip.x - 11 * Math.cos(angle - 0.42), tip.y - 11 * Math.sin(angle - 0.42));
    ctx.lineTo(tip.x - 11 * Math.cos(angle + 0.42), tip.y - 11 * Math.sin(angle + 0.42));
    ctx.closePath();
    ctx.fill();
  }

  // One glowing arc from a to b through the control point c, with its arrow at
  // b and a spark that travels the same way.
  function arc(a, b, c, rFrom, rTo, color, time, phase) {
    const start = toward(a, c, rFrom);
    const end = toward(b, c, rTo);
    ctx.strokeStyle = color;
    const alpha = ctx.globalAlpha;
    for (const [w, o] of [[9, 0.09], [4.5, 0.2], [1.5, 0.95]]) {
      ctx.globalAlpha = alpha * o;
      ctx.lineWidth = w;
      ctx.beginPath();
      ctx.moveTo(start.x, start.y);
      ctx.quadraticCurveTo(c.x, c.y, end.x, end.y);
      ctx.stroke();
    }
    ctx.globalAlpha = alpha;
    arrowHead(end, c, color);
    if (!still.matches) {
      const p = along(start, c, end, (time / 2200 + phase) % 1);
      ctx.drawImage(sprite(color), p.x - 9, p.y - 9, 18, 18);
      ctx.fillStyle = "#fff";
      ctx.beginPath();
      ctx.arc(p.x, p.y, 1.7, 0, Math.PI * 2);
      ctx.fill();
    }
  }

  function countTag(p, text, color) {
    ctx.font = "600 11px " + FONT;
    const w = ctx.measureText(text).width + 10;
    ctx.fillStyle = "rgba(5, 8, 26, 0.92)";
    ctx.strokeStyle = color;
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.rect(p.x - w / 2, p.y - 8, w, 16);
    ctx.fill();
    ctx.stroke();
    ctx.fillStyle = color;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(text, p.x, p.y + 0.5);
  }

  function drawSynapses(focus, time) {
    const tags = [];
    for (const e of shown.synapses) {
      const from = byKey.get(e.from);
      const to = byKey.get(e.to);
      const a = screen(layout().points.get(e.from));
      const b = screen(layout().points.get(e.to));
      const d = Math.hypot(b.x - a.x, b.y - a.y);
      if (d < 20) continue;
      const nx = -(b.y - a.y) / d;
      const ny = (b.x - a.x) / d;
      const bend = Math.min(46, d * 0.16);
      const control = (offset) => ({ x: (a.x + b.x) / 2 + nx * offset, y: (a.y + b.y) / 2 + ny * offset });
      const rFrom = scaled(from) + 4;
      const rTo = scaled(to) + 4;
      const phase = hash(e.from + e.to);
      const near = [e.from, e.to].some((k) => k === selected || k === hovered);
      ctx.globalAlpha = edgeAlpha(e.from, e.to, focus);
      if (e.direction === "two-way") {
        const one = control(bend + 13);
        const other = control(bend - 13);
        arc(a, b, one, rFrom, rTo, COLOR.twoway, time, phase);
        arc(b, a, other, rTo, rFrom, COLOR.twoway, time, phase + 0.5);
        if (near) {
          // Each count sits nearer the star its arrow reaches, so the two never touch.
          tags.push([along(a, one, b, 0.64), String(e.forward), COLOR.twoway]);
          tags.push([along(a, other, b, 0.36), String(e.back), COLOR.twoway]);
        }
      } else {
        const c = control(bend);
        arc(a, b, c, rFrom, rTo, COLOR.oneway, time, phase);
        if (near) tags.push([along(a, c, b, 0.5), String(e.forward), COLOR.oneway]);
      }
    }
    ctx.globalAlpha = 1;
    return tags;
  }

  function drawStars(focus, time) {
    for (const n of shown.nodes) {
      const p = screen(layout().points.get(n.key));
      const r = scaled(n);
      const color = colorOf(n);
      ctx.globalAlpha = focus && !focus.has(n.key) ? 0.22 : 1;
      if (n.state === "closed") {
        ctx.strokeStyle = color;
        ctx.lineWidth = 1.6;
        ctx.fillStyle = "#080d24";
        ctx.beginPath();
        ctx.arc(p.x, p.y, r * 0.75, 0, Math.PI * 2);
        ctx.fill();
        ctx.stroke();
      } else {
        const breathe = still.matches || n.state !== "live" ? 1 : 1 + Math.sin(time / 900 + hash(n.key) * 7) * 0.08;
        const glow = r * (n.id === null ? 3.4 : 5) * breathe;
        ctx.drawImage(sprite(color), p.x - glow, p.y - glow, glow * 2, glow * 2);
        if (isLead(n) || n.key === selected) {
          ctx.strokeStyle = color;
          ctx.lineWidth = 1;
          const alpha = ctx.globalAlpha;
          ctx.globalAlpha = alpha * 0.55;
          ctx.beginPath();
          ctx.moveTo(p.x - r * 2.6, p.y); ctx.lineTo(p.x + r * 2.6, p.y);
          ctx.moveTo(p.x, p.y - r * 2.6); ctx.lineTo(p.x, p.y + r * 2.6);
          ctx.stroke();
          ctx.globalAlpha = alpha;
        }
        ctx.fillStyle = color;
        ctx.beginPath();
        ctx.arc(p.x, p.y, r * 0.72, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = "rgba(255, 255, 255, 0.9)";
        ctx.beginPath();
        ctx.arc(p.x, p.y, r * 0.34, 0, Math.PI * 2);
        ctx.fill();
      }
      if (n.key === selected || n.key === hovered) {
        ctx.strokeStyle = n.key === selected ? "#ffffff" : "rgba(255, 255, 255, 0.55)";
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.arc(p.x, p.y, r + 6, 0, Math.PI * 2);
        ctx.stroke();
      }
    }
    ctx.globalAlpha = 1;
  }

  // Labels keep their size at every zoom. The more telling ones are placed
  // first and a label that would cover another is left out until zoomed in.
  function drawLabels(focus) {
    labelBoxes = [];
    const rank = (n) =>
      (n.key === selected ? 100 : 0) + (n.key === hovered ? 90 : 0) + (focus && focus.has(n.key) ? 50 : 0) +
      (isLead(n) ? 8 : 0) + (n.id !== null ? 4 : 0) + (n.state === "live" ? 2 : 0);
    const order = [...shown.nodes].sort((a, b) => rank(b) - rank(a));
    const small = layout().view.k < 0.6;
    ctx.textAlign = "center";
    ctx.textBaseline = "alphabetic";
    ctx.lineJoin = "round";
    for (const n of order) {
      const forced = n.key === selected || n.key === hovered;
      const p = screen(layout().points.get(n.key));
      const l = labelOf(n);
      // Zoomed out, the first line under the name (the folder) stays.
      const more = forced || !small ? l.more : l.more.slice(0, 1);
      const w = Math.max(textWidth(l.main, MAIN_FONT), ...more.map((m) => textWidth(m.text, SUB_FONT))) + 6;
      const top = p.y + scaled(n) + 5;
      const box = { key: n.key, x: p.x - w / 2, y: top, w, h: 15 + 13 * more.length };
      const covered = labelBoxes.some((o) => box.x < o.x + o.w && o.x < box.x + box.w && box.y < o.y + o.h && o.y < box.y + box.h);
      if (covered && !forced) continue;
      labelBoxes.push(box);
      ctx.globalAlpha = focus && !focus.has(n.key) ? 0.25 : 1;
      ctx.strokeStyle = "rgba(5, 8, 26, 0.92)";
      ctx.lineWidth = 3.5;
      ctx.font = MAIN_FONT;
      ctx.strokeText(l.main, p.x, top + 11);
      ctx.fillStyle = n.id === null ? "#b4bee6" : "#f1f4ff";
      ctx.fillText(l.main, p.x, top + 11);
      ctx.font = SUB_FONT;
      more.forEach((m, i) => {
        ctx.strokeText(m.text, p.x, top + 24 + 13 * i);
        ctx.fillStyle = m.color;
        ctx.fillText(m.text, p.x, top + 24 + 13 * i);
      });
    }
    ctx.globalAlpha = 1;
  }

  function frame(time) {
    const dpr = window.devicePixelRatio || 1;
    const w = window.innerWidth;
    const h = window.innerHeight;
    if (w !== width || h !== height || canvas.width !== Math.round(w * dpr)) {
      width = w;
      height = h;
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      paintBackdrop(dpr);
      if (data && !layout().moved) Object.assign(layout().view, fitView());
    }
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.drawImage(backdrop, 0, 0);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    if (!still.matches) {
      ctx.fillStyle = "#fff";
      for (const s of twinklers) {
        ctx.globalAlpha = Math.max(0, Math.sin(time / 1300 + s.phase)) * 0.55;
        ctx.beginPath();
        ctx.arc(s.x, s.y, s.r + 0.3, 0, Math.PI * 2);
        ctx.fill();
      }
      ctx.globalAlpha = 1;
    }
    const l = layout();
    if (l.alpha > 0 && shown.nodes.length) {
      step(l, shown, l.alpha, true);
      l.alpha *= 0.97;
      if (l.alpha < 0.012) {
        l.alpha = 0;
        for (const p of l.points.values()) { p.fresh = false; p.vx = 0; p.vy = 0; }
        if (!l.moved) flyTo(fitView());
      }
    }
    if (flight) {
      const t = clamp((performance.now() - flight.start) / 450, 0, 1);
      const ease = 1 - Math.pow(1 - t, 3);
      for (const k of ["x", "y", "k"]) l.view[k] = flight.from[k] + (flight.to[k] - flight.from[k]) * ease;
      if (t === 1) flight = null;
    }
    if (shown.nodes.length) {
      const focus = focusSet();
      drawHierarchy(focus);
      const tags = drawSynapses(focus, time);
      drawStars(focus, time);
      drawLabels(focus);
      for (const tag of tags) countTag(...tag);
    }
    requestAnimationFrame(frame);
  }

  // ---------- the page around the sky ----------

  const element = (tag, text, className) => {
    const e = document.createElement(tag);
    if (text !== undefined) e.textContent = text;
    if (className) e.className = className;
    return e;
  };

  const plural = (n, word) => n + " " + word + (n === 1 ? "" : "s");

  function when(last) {
    if (!last) return "time unknown";
    const t = new Date(last);
    return Number.isNaN(t.getTime()) ? last : "last " + t.toLocaleString();
  }

  function matches(n, q) {
    return [n.name, n.title, n.folder, n.repo, n.handle, n.id, n.build, n.role, n.type, n.harness, n.state, n.worktree, n.branch]
      .some((v) => v && v.toLowerCase().includes(q));
  }

  let noticeShape = "";
  function renderNotices() {
    const list = [...(data ? data.notices : [])];
    const why = (data && data.project_empty) || "";
    const shape = JSON.stringify([why, list, lost]);
    if (shape === noticeShape) return;
    noticeShape = shape;
    const lines = list.map((text) => element("p", text));
    if (why) lines.unshift(element("p", why, "info"));
    if (lost) lines.push(element("p", lost, "lost"));
    $("notices").replaceChildren(...lines);
  }

  function renderChrome() {
    const two = shown.synapses.filter((e) => e.direction === "two-way").length;
    $("counts").textContent = data
      ? plural(shown.nodes.length, "session") + " · " + plural(shown.synapses.length, "synapse") + (two ? " (" + two + " two-way)" : "")
      : "";
    const empty = $("empty");
    empty.hidden = shown.nodes.length > 0;
    empty.textContent = !data ? "Loading sessions…"
      : scope === "project" && data.nodes.length ? "No session of this project. All Orca sessions shows the rest."
      : "No sessions to show.";
  }

  function recompute() {
    const nodes = data ? data.nodes.filter((n) => scope === "all" || n.project) : [];
    const keys = new Set(nodes.map((n) => n.key));
    shown = {
      nodes,
      hierarchy: data ? data.hierarchy.filter((e) => keys.has(e.parent) && keys.has(e.child)) : [],
      synapses: data ? data.synapses.filter((e) => keys.has(e.from) && keys.has(e.to)) : [],
    };
    arrange();
    renderChrome();
  }

  // The scope the page opens with is decided once, by the first snapshot,
  // unless the person chose one before it arrived.
  let scopeSettled = false;

  function showScope(next) {
    scope = next;
    for (const b of document.querySelectorAll("[data-scope]")) b.setAttribute("aria-pressed", String(b.dataset.scope === scope));
  }

  function setScope(next) {
    scopeSettled = true;
    if (next === scope) return;
    showScope(next);
    flight = null;
    recompute();
    if (!layout().moved) Object.assign(layout().view, fitView());
    renderDetails();
  }

  function select(key) {
    selected = key;
    $("go-status").textContent = "";
    $("go-status").className = "";
    detailsShape = "";
    renderDetails();
  }

  // Selects a session and brings it to the middle of the free part of the sky.
  function reveal(key) {
    const n = byKey.get(key);
    if (!n) return;
    if (scope === "project" && !n.project) setScope("all");
    select(key);
    const p = layout().points.get(key);
    if (!p) return;
    const a = area();
    const k = Math.max(layout().view.k, 1);
    layout().moved = true;
    flyTo({ k, x: a.left + a.width / 2 - p.x * k, y: a.top + a.height / 2 - p.y * k });
  }

  function copyButton(value, source) {
    const b = element("button", "Copy");
    b.type = "button";
    b.addEventListener("click", () => {
      const done = () => { b.textContent = "Copied"; setTimeout(() => { b.textContent = "Copy"; }, 1200); };
      const choose = () => {
        const range = document.createRange();
        range.selectNodeContents(source);
        getSelection().removeAllRanges();
        getSelection().addRange(range);
        b.textContent = "Selected";
      };
      if (navigator.clipboard) navigator.clipboard.writeText(value).then(done, choose);
      else choose();
    });
    return b;
  }

  const nameOf = (key) => {
    const n = byKey.get(key);
    if (!n) return key;
    const main = labelOf(n).main;
    if (n.id !== null) return main + " · " + n.id.slice(-4);
    return n.folder && (n.title || n.name) ? main + " · " + n.folder : main;
  };

  function linkItem(key, title, detail, className) {
    const li = element("li", undefined, className);
    const b = element("button");
    b.type = "button";
    b.append(element("span", title));
    if (detail) b.append(element("small", detail));
    b.addEventListener("click", () => reveal(key));
    li.append(b);
    return li;
  }

  function renderDetails() {
    const panel = $("details");
    const n = selected && byKey.get(selected);
    if (!n) {
      panel.hidden = true;
      return;
    }
    panel.hidden = false;
    const synapses = data.synapses.filter((e) => e.from === n.key || e.to === n.key);
    const parents = data.hierarchy.filter((e) => e.child === n.key).map((e) => e.parent);
    const children = data.hierarchy.filter((e) => e.parent === n.key).map((e) => e.child);
    // Rebuilt only on a change, so a refresh does not take the focus or the
    // text the person selected.
    const shape = JSON.stringify([n, synapses, parents.map(nameOf), children.map(nameOf)]);
    if (shape === detailsShape) return;
    detailsShape = shape;

    $("d-title").textContent = titleOf(n);
    $("d-state").textContent = [n.state, n.id === null ? "no launch explains this terminal" : "launched by jitso"].join(" · ");
    const go = $("go");
    go.disabled = !n.listed;
    go.title = n.listed ? "Show this terminal in Orca" : "Orca lists no terminal for this session";

    const facts = [];
    const fact = (label, value, copy) => {
      if (value === null || value === "") return;
      const dd = element("dd");
      const span = element("span", value);
      dd.append(span);
      if (copy) dd.append(copyButton(value, span));
      facts.push(element("dt", label), dd);
    };
    fact("Session name", n.name, true);
    fact("Terminal tab", n.title);
    fact("Terminal handle", n.handle, true);
    fact("Launch id", n.id, true);
    fact("Role / type", [n.role, n.type].filter(Boolean).join(" / "));
    fact("Build", n.build);
    fact("Harness", n.harness);
    fact("Mode", n.mode);
    fact("Repository", n.repo || null);
    fact("Folder", n.folder);
    fact("Worktree", n.worktree);
    fact("Branch", n.branch);
    fact("In this project", n.project ? "yes" : "no");
    $("d-facts").replaceChildren(...facts);

    const sections = [];
    const section = (title, items, none) => {
      if (!items.length && !none) return;
      sections.push(element("h3", title));
      if (!items.length) { sections.push(element("p", none)); return; }
      const ul = element("ul");
      ul.append(...items);
      sections.push(ul);
    };
    section("Synapses", synapses.map((e) => {
      const mine = e.from === n.key;
      const other = mine ? e.to : e.from;
      const sent = mine ? e.forward : e.back;
      const received = mine ? e.back : e.forward;
      const two = e.direction === "two-way";
      const verb = two ? "⇄ " : sent ? "→ to " : "← from ";
      const count = two ? sent + " sent, " + received + " received" : plural(sent || received, "message") + (sent ? " sent" : " received");
      return linkItem(other, verb + nameOf(other), count + " · " + when(e.last), two ? "twoway" : "oneway");
    }), "This session has not talked with another one.");
    section("Parent", parents.map((k) => linkItem(k, nameOf(k))));
    section("Children", children.map((k) => linkItem(k, nameOf(k))));
    $("d-links").replaceChildren(...sections);
  }

  async function goToSession() {
    const n = selected && byKey.get(selected);
    if (!n || !n.listed) return;
    const status = $("go-status");
    status.className = "";
    status.textContent = "Switching…";
    try {
      const res = await fetch("api/focus", {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Jitso-Token": token },
        body: JSON.stringify({ handle: n.handle }),
      });
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || "HTTP " + res.status);
      status.textContent = "Orca now shows this terminal.";
    } catch (err) {
      status.className = "bad";
      status.textContent = "Could not switch: " + err.message;
    }
  }

  // ---------- search ----------

  function found() {
    if (!data || !query) return [];
    const inScope = (n) => (scope === "all" || n.project ? 0 : 1);
    return data.nodes.filter((n) => matches(n, query)).sort((a, b) => inScope(a) - inScope(b));
  }

  function renderResults() {
    const list = $("results");
    list.hidden = !query;
    if (!query) { list.replaceChildren(); return; }
    const hits = found();
    const items = hits.slice(0, 8).map((n, i) => {
      const li = element("li", undefined, i === 0 ? "first" : "");
      const b = element("button");
      b.type = "button";
      const where = scope === "project" && !n.project ? " · in All Orca sessions" : "";
      b.append(element("span", nameOf(n.key)));
      b.append(element("small", [n.name, n.folder, n.handle, n.state].filter(Boolean).join(" · ") + where));
      b.addEventListener("click", () => pick(n.key));
      li.append(b);
      return li;
    });
    if (!hits.length) items.push(element("p", "No session matches."));
    else if (hits.length > 8) items.push(element("p", hits.length - 8 + " more: keep typing."));
    list.replaceChildren(...items);
  }

  function pick(key) {
    $("search").value = "";
    query = "";
    renderResults();
    reveal(key);
    canvas.focus({ preventScroll: true });
  }

  $("search").addEventListener("input", (e) => {
    query = e.target.value.trim().toLowerCase();
    renderResults();
  });
  $("search").addEventListener("keydown", (e) => {
    if (e.key === "Enter" && found().length) pick(found()[0].key);
    if (e.key === "Escape") { e.target.value = ""; query = ""; renderResults(); }
  });

  // ---------- pointer and keys ----------

  function hit(x, y) {
    let best = null;
    let bestDistance = Infinity;
    for (const n of shown.nodes) {
      const p = screen(layout().points.get(n.key));
      const d = Math.hypot(p.x - x, p.y - y);
      if (d < Math.max(scaled(n) + 8, 14) && d < bestDistance) { best = n.key; bestDistance = d; }
    }
    if (best) return best;
    const box = labelBoxes.find((b) => x >= b.x && x <= b.x + b.w && y >= b.y && y <= b.y + b.h);
    return box ? box.key : null;
  }

  function zoom(factor, x, y) {
    const v = layout().view;
    const k = clamp(v.k * factor, 0.15, 4);
    v.x = x - ((x - v.x) * k) / v.k;
    v.y = y - ((y - v.y) * k) / v.k;
    v.k = k;
    layout().moved = true;
    flight = null;
  }

  let drag = null;
  canvas.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    canvas.setPointerCapture(e.pointerId);
    drag = { x: e.clientX, y: e.clientY, startX: e.clientX, startY: e.clientY, key: hit(e.clientX, e.clientY), moved: false };
    flight = null;
    $("results").hidden = true;
  });
  canvas.addEventListener("pointermove", (e) => {
    if (!drag) {
      hovered = hit(e.clientX, e.clientY);
      canvas.style.cursor = hovered ? "pointer" : "grab";
      return;
    }
    if (!drag.moved && Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) < 4) return;
    drag.moved = true;
    canvas.style.cursor = "grabbing";
    if (drag.key) {
      // A dragged star stays where it is dropped; a double click frees it.
      const p = layout().points.get(drag.key);
      if (p) { Object.assign(p, world(e.clientX, e.clientY), { pinned: true }); }
    } else {
      layout().view.x += e.clientX - drag.x;
      layout().view.y += e.clientY - drag.y;
    }
    layout().moved = true;
    drag.x = e.clientX;
    drag.y = e.clientY;
  });
  const release = (e) => {
    if (drag && !drag.moved && e.type === "pointerup") select(drag.key);
    drag = null;
    canvas.style.cursor = "grab";
  };
  canvas.addEventListener("pointerup", release);
  canvas.addEventListener("pointercancel", release);
  canvas.addEventListener("pointerleave", () => { if (!drag) hovered = null; });
  canvas.addEventListener("dblclick", (e) => {
    const p = layout().points.get(hit(e.clientX, e.clientY));
    if (p && p.pinned) { p.pinned = false; layout().alpha = 0.15; }
  });
  canvas.addEventListener("wheel", (e) => {
    e.preventDefault();
    zoom(Math.exp(-e.deltaY * (e.deltaMode === 1 ? 0.05 : 0.0016)), e.clientX, e.clientY);
  }, { passive: false });

  function fit() {
    layout().moved = false;
    flyTo(fitView());
  }

  document.addEventListener("keydown", (e) => {
    if (e.target instanceof HTMLInputElement || e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key === "Escape") select(null);
    else if (e.key === "+" || e.key === "=") zoom(1.25, width / 2, height / 2);
    else if (e.key === "-") zoom(0.8, width / 2, height / 2);
    else if (e.key === "0") fit();
    else if (e.key === "/") { e.preventDefault(); $("search").focus(); }
  });
  $("fit").addEventListener("click", fit);
  $("close").addEventListener("click", () => select(null));
  $("go").addEventListener("click", goToSession);
  for (const b of document.querySelectorAll("[data-scope]")) b.addEventListener("click", () => setScope(b.dataset.scope));

  // ---------- live updates ----------

  let timer = 0;
  let polling = false;
  async function poll() {
    clearTimeout(timer);
    if (polling) return;
    polling = true;
    if (!document.hidden) {
      try {
        const res = await fetch("api/snapshot", { cache: "no-store" });
        if (!res.ok) throw new Error((await res.json().catch(() => ({}))).error || "HTTP " + res.status);
        const next = await res.json();
        data = next;
        byKey = new Map(data.nodes.map((n) => [n.key, n]));
        lost = "";
        if (!scopeSettled) {
          scopeSettled = true;
          if (!data.nodes.some((n) => n.project)) showScope("all");
        }
        recompute();
        renderDetails();
        if (query) renderResults();
      } catch (err) {
        lost = err instanceof TypeError
          ? "The connection to jitso graph is lost. Is it still running? After a restart, reload the page."
          : "The snapshot could not be read: " + err.message;
      }
      renderNotices();
    }
    polling = false;
    timer = setTimeout(poll, POLL_MS);
  }
  document.addEventListener("visibilitychange", () => { if (!document.hidden) poll(); });

  renderChrome();
  requestAnimationFrame((t) => { frame(t); poll(); });
})();
