import Graph from "graphology";
import type { KGEntity, KGRelation } from "@/types/knowledge-graph";
import type { KGGraphNode, KGGraphEdge } from "@/types/graph-dto";
import { getNodeSize, truncateMiddle } from "@/components/graph/graph-utils";

// Solid colors per entity type
export const KG_TYPE_COLORS: Record<string, string> = {
  person: "#E85D24", organization: "#ef4444", project: "#22c55e",
  product: "#f97316", technology: "#3b82f6", task: "#f59e0b",
  event: "#ec4899", document: "#8b5cf6", concept: "#a78bfa", location: "#14b8a6",
};
export const KG_DEFAULT_COLOR = "#9ca3af";

function safeString(value: unknown, fallback = ""): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}

/** Compute degree (edge count) for each entity id. */
export function computeDegreeMap(entities: KGEntity[], relations: KGRelation[]): Map<string, number> {
  const deg = new Map<string, number>();
  const ids = new Set(entities.map((e) => e.id));
  for (const r of relations) {
    if (ids.has(r.source_entity_id)) deg.set(r.source_entity_id, (deg.get(r.source_entity_id) ?? 0) + 1);
    if (ids.has(r.target_entity_id)) deg.set(r.target_entity_id, (deg.get(r.target_entity_id) ?? 0) + 1);
  }
  return deg;
}

/** Build KG graph from compact DTOs. */
export function buildKGGraphFromDTO(nodes: KGGraphNode[], edges: KGGraphEdge[]): Graph {
  const graph = new Graph({ multi: false, type: "directed" });
  const nodeIds = new Set<string>();
  for (const n of nodes) {
    const id = safeString(n.id);
    if (id) nodeIds.add(id);
  }

  // Pre-compute degree from edges (compact DTO doesn't include it)
  const deg = new Map<string, number>();
  for (const e of edges) {
    const src = safeString(e.src);
    const tgt = safeString(e.tgt);
    if (nodeIds.has(src)) deg.set(src, (deg.get(src) ?? 0) + 1);
    if (nodeIds.has(tgt)) deg.set(tgt, (deg.get(tgt) ?? 0) + 1);
  }

  for (const n of nodes) {
    const id = safeString(n.id);
    if (!id || graph.hasNode(id)) continue;
    const entityType = safeString(n.t, "concept");
    graph.addNode(id, {
      label: truncateMiddle(safeString(n.n, id.slice(0, 8)), 28),
      x: 0, y: 0,
      size: getNodeSize(deg.get(id) ?? 0, nodes.length),
      color: KG_TYPE_COLORS[entityType] ?? KG_DEFAULT_COLOR,
      entityType,
    });
  }

  for (const e of edges) {
    const src = safeString(e.src);
    const tgt = safeString(e.tgt);
    if (!src || !tgt || !nodeIds.has(src) || !nodeIds.has(tgt) || graph.hasEdge(src, tgt)) continue;
    const edgeId = safeString(e.id, `${src}->${tgt}`);
    if (!graph.hasEdge(edgeId)) {
      graph.addEdgeWithKey(edgeId, src, tgt, {
        label: safeString(e.type, "related").replace(/_/g, " "), type: "curvedArrow",
      });
    }
  }

  return graph;
}

/** Build a Graphology graph from KG entities and relations. */
export function buildKGGraph(entities: KGEntity[], allRelations: KGRelation[]): Graph {
  const graph = new Graph({ multi: false, type: "directed" });
  const entityIds = new Set(entities.map((e) => safeString(e.id)).filter(Boolean));
  const degreeMap = computeDegreeMap(entities, allRelations);

  // Add nodes (x/y assigned by container via circular layout before FA2)
  for (const e of entities) {
    const id = safeString(e.id);
    if (id && !graph.hasNode(id)) {
      const degree = degreeMap.get(id) ?? 0;
      const entityType = safeString(e.entity_type, "concept");
      graph.addNode(id, {
        label: truncateMiddle(safeString(e.name, id.slice(0, 8)), 28),
        x: 0,
        y: 0,
        size: getNodeSize(degree, entities.length),
        color: KG_TYPE_COLORS[entityType] ?? KG_DEFAULT_COLOR,
        entityType,
      });
    }
  }

  // Add edges (straight arrows for KG)
  for (const r of allRelations) {
    const source = safeString(r.source_entity_id);
    const target = safeString(r.target_entity_id);
    if (source && target && entityIds.has(source) && entityIds.has(target) && !graph.hasEdge(source, target)) {
      const edgeId = safeString(r.id, `${source}->${target}`);
      if (!graph.hasEdge(edgeId)) {
        graph.addEdgeWithKey(edgeId, source, target, {
          label: safeString(r.relation_type, "related").replace(/_/g, " "),
          type: "curvedArrow",
        });
      }
    }
  }

  return graph;
}

/** Limit entities to nodeLimit by degree centrality (highest-degree first). */
export function limitEntitiesByDegree(
  allEntities: KGEntity[],
  allRelations: KGRelation[],
  nodeLimit: number,
): KGEntity[] {
  if (allEntities.length <= nodeLimit) return allEntities;
  const deg = computeDegreeMap(allEntities, allRelations);
  return [...allEntities].sort((a, b) => (deg.get(b.id) ?? 0) - (deg.get(a.id) ?? 0)).slice(0, nodeLimit);
}
