import { memo } from "react";
import { Handle, Position, type NodeProps } from "reactflow";
import { Repeat, Rows3 } from "lucide-react";
import { cn } from "@/lib/utils";

export interface ContainerNodeData {
  kind: "loop" | "parallel";
  label: string;
  // loop config
  loopType?: "for" | "forEach" | "while" | "doWhile";
  iterations?: number;
  forEachItems?: string;
  whileCondition?: string;
  doWhileCondition?: string;
  // parallel config
  parallelType?: "count" | "collection";
  count?: number;
  distribution?: string;
  batchSize?: number;
}

// ContainerNode renders a loop/parallel subflow as a translucent group box.
// reactflow positions child nodes (parentNode + extent:"parent") inside it; the
// box only provides the visual boundary, header, and subflow config summary.
// Its loop_exit / parallel_exit handle on the right is where downstream edges
// attach after the subflow completes.
export const ContainerNode = memo(({ data, selected }: NodeProps<ContainerNodeData>) => {
  const isLoop = data.kind === "loop";
  const Icon = isLoop ? Repeat : Rows3;
  const accent = isLoop ? "border-violet-400/70 bg-violet-500/5" : "border-cyan-400/70 bg-cyan-500/5";

  return (
    <div
      className={cn(
        "h-full w-full rounded-lg border-2 border-dashed",
        accent,
        selected ? "ring-2 ring-primary" : "",
      )}
    >
      <Handle type="target" position={Position.Left} className="!h-2.5 !w-2.5 !bg-muted-foreground" />
      <div className="flex items-center gap-1.5 rounded-t-md border-b border-dashed px-2 py-1 text-xs font-medium">
        <Icon className="h-3.5 w-3.5" />
        <span className="truncate">{data.label || (isLoop ? "循环" : "并行")}</span>
        <span className="ml-auto text-[10px] text-muted-foreground">{subflowSummary(data)}</span>
      </div>
      <Handle
        id={isLoop ? "loop_exit" : "parallel_exit"}
        type="source"
        position={Position.Right}
        className="!h-2.5 !w-2.5 !bg-primary"
      />
    </div>
  );
});

ContainerNode.displayName = "ContainerNode";

// subflowSummary renders a compact config hint in the container header.
function subflowSummary(d: ContainerNodeData): string {
  if (d.kind === "loop") {
    switch (d.loopType) {
      case "forEach":
        return "forEach";
      case "while":
        return "while";
      case "doWhile":
        return "doWhile";
      default:
        return `for ×${d.iterations ?? 1}`;
    }
  }
  return d.parallelType === "collection" ? "collection" : `count ×${d.count ?? 1}`;
}
