import { useCallback, useMemo, useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  type Connection,
  type Node,
  type NodeTypes,
} from "reactflow";
import "reactflow/dist/style.css";
import { Save, Play, Plus, Repeat, Rows3 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { ROUTES } from "@/lib/routes";
import { BlockNode, type BlockNodeData } from "./components/block-node";
import { ContainerNode, type ContainerNodeData } from "./components/container-node";
import { ContainerConfigPanel } from "./components/container-config-panel";
import { NodeConfigPanel } from "./components/node-config-panel";
import { NODE_TYPES } from "@/types/workflow-graph";
import { toGraph, fromGraph, CONTAINER_SIZE, type AnyNodeData } from "./lib/graph-convert";
import { useGraphDefinitions } from "./hooks/use-graph-definitions";

const nodeTypes: NodeTypes = { block: BlockNode, container: ContainerNode };

let idSeq = 1;
const nextId = () => `n${Date.now()}_${idSeq++}`;

// absolutePosition resolves a node's position to canvas coordinates, accounting
// for a parent container offset (reactflow child positions are parent-relative).
function absolutePosition(n: Node<AnyNodeData>, all: Node<AnyNodeData>[]): { x: number; y: number } {
  if (!n.parentNode) return { x: n.position.x, y: n.position.y };
  const parent = all.find((p) => p.id === n.parentNode);
  if (!parent) return { x: n.position.x, y: n.position.y };
  return { x: parent.position.x + n.position.x, y: parent.position.y + n.position.y };
}

export function WorkflowBuilderPage() {
  useTranslation("sidebar");
  const { id: editId } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { getDefinition, saveDefinition, runDefinition } = useGraphDefinitions();

  const [nodes, setNodes, onNodesChange] = useNodesState<AnyNodeData>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState([]);
  const [name, setName] = useState("新建工作流");
  const [defId, setDefId] = useState<string | undefined>(editId);
  const [saving, setSaving] = useState(false);
  const [running, setRunning] = useState(false);
  const [runResult, setRunResult] = useState<string>("");
  const [selectedId, setSelectedId] = useState<string | undefined>();

  // Load existing definition when editing.
  useEffect(() => {
    if (!editId) return;
    getDefinition(editId).then((def) => {
      setName(def.name);
      setDefId(def.id);
      const { nodes: n, edges: e } = fromGraph(def.graph);
      setNodes(n);
      setEdges(e);
    });
  }, [editId, getDefinition, setNodes, setEdges]);

  const onConnect = useCallback(
    (conn: Connection) => setEdges((eds) => addEdge({ ...conn, sourceHandle: conn.sourceHandle ?? "source" }, eds)),
    [setEdges],
  );

  const addNode = useCallback(
    (type: string, label: string) => {
      const node: Node<AnyNodeData> = {
        id: nextId(),
        type: "block",
        position: { x: 120 + Math.random() * 240, y: 80 + Math.random() * 240 },
        data: { type, label } as BlockNodeData,
      };
      setNodes((nds) => [...nds, node]);
    },
    [setNodes],
  );

  // addContainer drops a loop/parallel group box on the canvas. Blocks dragged
  // inside it become its members (see onNodeDragStop).
  const addContainer = useCallback(
    (kind: "loop" | "parallel") => {
      const node: Node<AnyNodeData> = {
        id: nextId(),
        type: "container",
        position: { x: 80 + Math.random() * 120, y: 80 + Math.random() * 120 },
        style: { width: CONTAINER_SIZE.width, height: CONTAINER_SIZE.height },
        data:
          kind === "loop"
            ? ({ kind: "loop", label: "循环", loopType: "for", iterations: 3 } as ContainerNodeData)
            : ({ kind: "parallel", label: "并行", parallelType: "count", count: 2 } as ContainerNodeData),
      };
      // containers must precede children in the array
      setNodes((nds) => [node, ...nds]);
    },
    [setNodes],
  );

  // onNodeDragStop reparents a block into a container when it is dropped inside
  // the container's bounds (and detaches it when dragged out). Mirrors sim's
  // drag-into-subflow behavior. Containers themselves are never reparented.
  const onNodeDragStop = useCallback(
    (_e: unknown, dragged: Node<AnyNodeData>) => {
      if (dragged.type === "container") return;
      setNodes((nds) => {
        const containers = nds.filter((n) => n.type === "container");
        // absolute position of the dragged node
        const abs = absolutePosition(dragged, nds);
        let target: Node<AnyNodeData> | undefined;
        for (const c of containers) {
          const cpos = absolutePosition(c, nds);
          const w = (c.style?.width as number) ?? CONTAINER_SIZE.width;
          const h = (c.style?.height as number) ?? CONTAINER_SIZE.height;
          if (abs.x >= cpos.x && abs.x <= cpos.x + w && abs.y >= cpos.y && abs.y <= cpos.y + h) {
            target = c;
            break;
          }
        }
        return nds.map((n) => {
          if (n.id !== dragged.id) return n;
          if (target) {
            const cpos = absolutePosition(target, nds);
            return {
              ...n,
              parentNode: target.id,
              extent: "parent" as const,
              position: { x: abs.x - cpos.x, y: abs.y - cpos.y },
            };
          }
          // dropped outside any container: detach
          if (n.parentNode) {
            return { ...n, parentNode: undefined, extent: undefined, position: abs };
          }
          return n;
        });
      });
    },
    [setNodes],
  );

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      const graph = toGraph(nodes, edges);
      const saved = await saveDefinition({ id: defId, name, graph });
      setDefId(saved.id);
      if (!editId) navigate(ROUTES.WORKFLOW_BUILDER_EDIT.replace(":id", saved.id), { replace: true });
    } finally {
      setSaving(false);
    }
  }, [nodes, edges, name, defId, editId, saveDefinition, navigate]);

  const handleRun = useCallback(async () => {
    if (!defId) return;
    setRunning(true);
    setRunResult("");
    try {
      const run = await runDefinition(defId, {});
      setRunResult(`状态: ${run.status}\n${JSON.stringify(run.output ?? {}, null, 2)}`);
    } catch (err) {
      setRunResult(`运行失败: ${(err as Error).message}`);
    } finally {
      setRunning(false);
    }
  }, [defId, runDefinition]);

  const palette = useMemo(() => NODE_TYPES, []);

  // The selected container (if a container node is selected) and a patcher that
  // merges config changes back into its node data.
  const selectedContainer = useMemo(() => {
    const n = nodes.find((x) => x.id === selectedId);
    return n && n.type === "container" ? (n as Node<ContainerNodeData>) : undefined;
  }, [nodes, selectedId]);

  const patchContainer = useCallback(
    (patch: Partial<ContainerNodeData>) => {
      setNodes((nds) =>
        nds.map((n) =>
          n.id === selectedId ? { ...n, data: { ...(n.data as ContainerNodeData), ...patch } } : n,
        ),
      );
    },
    [selectedId, setNodes],
  );

  // The selected block node (non-container) and its patcher.
  const selectedBlock = useMemo(() => {
    const n = nodes.find((x) => x.id === selectedId);
    return n && n.type === "block" ? (n as Node<BlockNodeData>) : undefined;
  }, [nodes, selectedId]);

  const patchBlock = useCallback(
    (patch: Partial<BlockNodeData>) => {
      setNodes((nds) =>
        nds.map((n) =>
          n.id === selectedId ? { ...n, data: { ...(n.data as BlockNodeData), ...patch } } : n,
        ),
      );
    },
    [selectedId, setNodes],
  );

  const onNodeClick = useCallback((_e: unknown, node: Node<AnyNodeData>) => setSelectedId(node.id), []);
  const onPaneClick = useCallback(() => setSelectedId(undefined), []);

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="工作流编辑器" />
      <div className="flex items-center gap-2 border-b px-4 py-2">
        <input
          className="rounded border bg-background px-2 py-1 text-sm"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="工作流名称"
        />
        <Button size="sm" onClick={handleSave} disabled={saving}>
          <Save className="mr-1 h-4 w-4" />
          {saving ? "保存中…" : "保存"}
        </Button>
        <Button size="sm" variant="outline" onClick={handleRun} disabled={!defId || running}>
          <Play className="mr-1 h-4 w-4" />
          {running ? "运行中…" : "运行"}
        </Button>
      </div>

      <div className="flex min-h-0 flex-1">
        {/* node palette */}
        <div className="w-44 shrink-0 space-y-1 overflow-y-auto border-r p-2">
          <div className="px-1 pb-1 text-xs font-medium text-muted-foreground">节点</div>
          {palette.map((p) => (
            <button
              key={p.type}
              onClick={() => addNode(p.type, p.label)}
              className="flex w-full items-center gap-1 rounded border px-2 py-1.5 text-left text-sm hover:bg-accent"
            >
              <Plus className="h-3 w-3" />
              {p.label}
            </button>
          ))}
          <div className="px-1 pb-1 pt-3 text-xs font-medium text-muted-foreground">容器</div>
          <button
            onClick={() => addContainer("loop")}
            className="flex w-full items-center gap-1 rounded border px-2 py-1.5 text-left text-sm hover:bg-accent"
          >
            <Repeat className="h-3 w-3" />
            循环
          </button>
          <button
            onClick={() => addContainer("parallel")}
            className="flex w-full items-center gap-1 rounded border px-2 py-1.5 text-left text-sm hover:bg-accent"
          >
            <Rows3 className="h-3 w-3" />
            并行
          </button>
        </div>

        {/* canvas */}
        <div className="relative min-w-0 flex-1">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={onNodesChange}
            onEdgesChange={onEdgesChange}
            onConnect={onConnect}
            onNodeDragStop={onNodeDragStop}
            onNodeClick={onNodeClick}
            onPaneClick={onPaneClick}
            nodeTypes={nodeTypes}
            fitView
          >
            <Background />
            <Controls />
            <MiniMap />
          </ReactFlow>
          {selectedContainer ? (
            <ContainerConfigPanel
              data={selectedContainer.data}
              onChange={patchContainer}
              onClose={() => setSelectedId(undefined)}
            />
          ) : null}
          {selectedBlock ? (
            <NodeConfigPanel
              data={selectedBlock.data}
              onChange={patchBlock}
              onClose={() => setSelectedId(undefined)}
            />
          ) : null}
          {runResult ? (
            <pre className="absolute bottom-2 right-2 max-h-48 max-w-md overflow-auto rounded border bg-card p-2 text-xs shadow">
              {runResult}
            </pre>
          ) : null}
        </div>
      </div>
    </div>
  );
}
