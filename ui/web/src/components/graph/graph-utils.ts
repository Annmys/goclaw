import louvain from "graphology-communities-louvain";
import type Graph from "graphology";

/** 16-color high-contrast palette for community detection. */
const COMMUNITY_PALETTE = [
  "#e6194B", "#3cb44b", "#4363d8", "#f58231",
  "#42d4f4", "#f032e6", "#bfef45", "#fabed4",
  "#469990", "#dcbeff", "#9A6324", "#ffe119",
  "#800000", "#aaffc3", "#808000", "#000075",
] as const;

/** Run Louvain community detection and assign `community` + `color` attrs. */
export function assignCommunityColors(graph: Graph): void {
  if (graph.order === 0) return;
  const communities = louvain(graph, { resolution: 1.0 });
  graph.forEachNode((node) => {
    const c = communities[node] ?? 0;
    graph.setNodeAttribute(node, "community", c);
    graph.setNodeAttribute(node, "color", COMMUNITY_PALETTE[c % COMMUNITY_PALETTE.length]);
  });
}

/** Get community palette color by index. */
export function getCommunityColor(idx: number): string {
  if (!Number.isFinite(idx) || idx < 0) idx = 0;
  return COMMUNITY_PALETTE[idx % COMMUNITY_PALETTE.length]!;
}

/** Degree-based node sizing, scaled by graph density. */
export function getNodeSize(degree: number, nodeCount = 200): number {
  if (!Number.isFinite(degree) || degree < 0) degree = 0;
  if (!Number.isFinite(nodeCount) || nodeCount < 1) nodeCount = 1;
  const s = nodeCount < 100 ? 1.0 : nodeCount < 500 ? 0.6 : nodeCount < 2000 ? 0.4 : 0.3;
  const base = 3 * s;
  if (degree === 0) return base;
  return base + Math.min(Math.log2(degree + 1) * 1.2 * s, 5 * s);
}

/** Truncate a long string by removing the middle and inserting an ellipsis. */
export function truncateMiddle(str: unknown, maxLength = 28): string {
  if (typeof str !== "string") return "";
  if (!Number.isFinite(maxLength) || maxLength < 4) maxLength = 4;
  if (!str || str.length <= maxLength) return str;
  const keepStart = Math.ceil((maxLength - 3) * 0.6);
  const keepEnd = Math.floor((maxLength - 3) * 0.4);
  return `${str.slice(0, keepStart)}...${str.slice(-keepEnd)}`;
}

/** Adaptive FA2 worker config based on graph size and orphan ratio. */
export function getFA2WorkerSettings(nodeCount: number, orphanRatio: number) {
  if (!Number.isFinite(nodeCount) || nodeCount < 1) nodeCount = 1;
  if (!Number.isFinite(orphanRatio) || orphanRatio < 0) orphanRatio = 0;
  orphanRatio = Math.min(orphanRatio, 1);

  const baseScaling = nodeCount < 200 ? 5 : nodeCount < 1000 ? 10 : 20;
  return {
    settings: {
      linLogMode: false,
      outboundAttractionDistribution: true,
      gravity: 0.5 + orphanRatio * 2.0,
      scalingRatio: baseScaling + (1 - orphanRatio) * 5,
      strongGravityMode: false,
      slowDown: 5,
      barnesHutOptimize: nodeCount > 50,
      barnesHutTheta: 0.5,
      edgeWeightInfluence: 0,
      adjustSizes: true,
    },
    durationMs: nodeCount < 200 ? 2000 :
                nodeCount < 1000 ? 3500 :
                nodeCount < 5000 ? 5000 : 8000,
  };
}

/** Semantic zoom tiers: cameraRatio thresholds. Lower means more zoomed in. */
export const ZOOM_TIERS = {
  FAR: 0.6,
  MID: 0.3,
  NEAR: 0.12,
} as const;

/** Minimum degree for nodes to remain visible per tier. */
export const TIER_MIN_DEGREE = {
  FAR: 2,
  MID: 1,
  NEAR: 0,
} as const;

/** Minimum endpoint degree for edges per tier. */
export const TIER_EDGE_DEGREE = {
  FAR: Infinity,
  MID: 6,
  NEAR: 2,
} as const;

/** Sigma settings constants for consistent look across graph views. */
export const SIGMA_SETTINGS = {
  labelRenderedSizeThreshold: 14,
  labelDensity: 0.04,
  labelGridCellSize: 200,
  defaultEdgeColor: "#334155",
  minCameraRatio: 0.02,
  maxCameraRatio: 8,
} as const;
