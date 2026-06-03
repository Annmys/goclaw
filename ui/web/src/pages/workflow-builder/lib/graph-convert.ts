import type { Edge, Node } from "reactflow";
import type { BlockNodeData } from "../components/block-node";
import type { ContainerNodeData } from "../components/container-node";
import {
  GRAPH_VERSION,
  type GraphBlock,
  type GraphConnection,
  type GraphLoop,
  type GraphParallel,
  type WorkflowGraph,
} from "@/types/workflow-graph";

// Default size for a container node when first created / when restored.
export const CONTAINER_SIZE = { width: 360, height: 220 };

// AnyNode is either a block node or a loop/parallel container node.
export type AnyNodeData = BlockNodeData | ContainerNodeData;

function isContainer(n: Node<AnyNodeData>): n is Node<ContainerNodeData> {
  return n.type === "container";
}

// toGraph converts the reactflow canvas (block nodes, container nodes, edges)
// into the backend's serialized WorkflowGraph. Container nodes do NOT become
// blocks; instead each contributes a loops[]/parallels[] entry whose `nodes`
// are the ids of its child blocks (reactflow parentNode === container id).
export function toGraph(nodes: Node<AnyNodeData>[], edges: Edge[]): WorkflowGraph {
  const blocks: GraphBlock[] = [];
  const loops: Record<string, GraphLoop> = {};
  const parallels: Record<string, GraphParallel> = {};

  // group child block ids by their parent container
  const childrenOf: Record<string, string[]> = {};
  for (const n of nodes) {
    if (n.parentNode) {
      (childrenOf[n.parentNode] ??= []).push(n.id);
    }
  }

  for (const n of nodes) {
    if (isContainer(n)) {
      const members = childrenOf[n.id] ?? [];
      const d = n.data;
      if (d.kind === "loop") {
        loops[n.id] = {
          id: n.id,
          nodes: members,
          loopType: d.loopType ?? "for",
          iterations: d.iterations,
          forEachItems: d.forEachItems,
          whileCondition: d.whileCondition,
          doWhileCondition: d.doWhileCondition,
        };
      } else {
        parallels[n.id] = {
          id: n.id,
          nodes: members,
          parallelType: d.parallelType ?? "count",
          count: d.count,
          distribution: d.distribution,
          batchSize: d.batchSize,
        };
      }
      continue;
    }
    const b = n as Node<BlockNodeData>;
    blocks.push({
      id: b.id,
      position: { x: b.position.x, y: b.position.y },
      config: {
        tool: (b.data as any).tool ?? undefined,
        params: (b.data as any).params ?? {},
      },
      metadata: { id: b.data.type, name: b.data.label },
      enabled: true,
    });
  }

  const connections: GraphConnection[] = edges.map((e) => ({
    source: e.source,
    target: e.target,
    sourceHandle: e.sourceHandle ?? "source",
    targetHandle: e.targetHandle ?? undefined,
  }));

  return {
    version: GRAPH_VERSION,
    blocks,
    connections,
    loops: Object.keys(loops).length ? loops : undefined,
    parallels: Object.keys(parallels).length ? parallels : undefined,
  };
}

// fromGraph converts a stored WorkflowGraph back into reactflow nodes + edges.
// loops[]/parallels[] become container nodes; their member blocks are reparented
// to the container so the canvas redraws the grouping.
export function fromGraph(graph: WorkflowGraph): { nodes: Node<AnyNodeData>[]; edges: Edge[] } {
  const memberToContainer: Record<string, string> = {};
  const containers: Node<AnyNodeData>[] = [];

  for (const [id, l] of Object.entries(graph.loops ?? {})) {
    for (const m of l.nodes) memberToContainer[m] = id;
    containers.push({
      id,
      type: "container",
      position: { x: 40, y: 40 },
      style: { width: CONTAINER_SIZE.width, height: CONTAINER_SIZE.height },
      data: {
        kind: "loop",
        label: "循环",
        loopType: l.loopType ?? "for",
        iterations: l.iterations,
        forEachItems: typeof l.forEachItems === "string" ? l.forEachItems : undefined,
        whileCondition: l.whileCondition,
        doWhileCondition: l.doWhileCondition,
      } as ContainerNodeData,
    });
  }
  for (const [id, p] of Object.entries(graph.parallels ?? {})) {
    for (const m of p.nodes) memberToContainer[m] = id;
    containers.push({
      id,
      type: "container",
      position: { x: 40, y: 300 },
      style: { width: CONTAINER_SIZE.width, height: CONTAINER_SIZE.height },
      data: {
        kind: "parallel",
        label: "并行",
        parallelType: p.parallelType ?? "count",
        count: p.count,
        distribution: typeof p.distribution === "string" ? p.distribution : undefined,
        batchSize: p.batchSize,
      } as ContainerNodeData,
    });
  }

  const blockNodes: Node<AnyNodeData>[] = (graph.blocks ?? []).map((b) => {
    const parent = memberToContainer[b.id];
    return {
      id: b.id,
      type: "block",
      position: { x: b.position?.x ?? 0, y: b.position?.y ?? 0 },
      ...(parent ? { parentNode: parent, extent: "parent" as const } : {}),
      data: {
        type: b.metadata?.id ?? "agent",
        label: b.metadata?.name ?? "",
        tool: b.config?.tool,
        params: b.config?.params,
      } as BlockNodeData,
    };
  });

  // containers must come before their children in the array for reactflow.
  const nodes = [...containers, ...blockNodes];

  const edges: Edge[] = (graph.connections ?? []).map((c, i) => ({
    id: `e${i}-${c.source}-${c.target}`,
    source: c.source,
    target: c.target,
    sourceHandle: c.sourceHandle ?? "source",
    targetHandle: c.targetHandle ?? undefined,
  }));

  return { nodes, edges };
}
