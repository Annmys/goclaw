import { memo } from "react";
import { Handle, Position, type NodeProps } from "reactflow";
import { cn } from "@/lib/utils";

// node type → accent color (tailwind classes). Keeps the canvas readable.
const TYPE_COLORS: Record<string, string> = {
  trigger: "border-l-emerald-500",
  agent: "border-l-violet-500",
  tool: "border-l-blue-500",
  function: "border-l-amber-500",
  api: "border-l-cyan-500",
  condition: "border-l-orange-500",
  router: "border-l-pink-500",
  knowledge: "border-l-teal-500",
  response: "border-l-green-500",
  "human-in-the-loop": "border-l-red-500",
  wait: "border-l-slate-500",
};

export interface BlockNodeData {
  type: string;
  label: string;
  tool?: string;
  params?: Record<string, unknown>;
  selected?: boolean;
}

// BlockNode renders a single workflow block on the canvas with an inbound and
// outbound handle. Condition/router nodes expose typed source handles so the
// canvas can draw branch edges that map to the backend's condition-/router-
// handles.
export const BlockNode = memo(({ data, selected }: NodeProps<BlockNodeData>) => {
  const accent = TYPE_COLORS[data.type] ?? "border-l-gray-400";
  const isBranching = data.type === "condition" || data.type === "router";
  return (
    <div
      className={cn(
        "min-w-[160px] rounded-md border border-l-4 bg-card px-3 py-2 shadow-sm transition",
        accent,
        selected ? "ring-2 ring-primary" : "",
      )}
    >
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !bg-muted-foreground" />
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{data.type}</div>
      <div className="truncate text-sm font-semibold text-foreground">{data.label || "未命名"}</div>
      {isBranching ? (
        <>
          <Handle id="source" type="source" position={Position.Right} style={{ top: "40%" }} className="!h-2 !w-2 !bg-primary" />
          <Handle id="alt" type="source" position={Position.Right} style={{ top: "70%" }} className="!h-2 !w-2 !bg-orange-400" />
        </>
      ) : (
        <Handle id="source" type="source" position={Position.Right} className="!h-2 !w-2 !bg-primary" />
      )}
    </div>
  );
});

BlockNode.displayName = "BlockNode";
