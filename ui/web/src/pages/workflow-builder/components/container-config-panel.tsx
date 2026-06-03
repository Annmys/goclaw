import type { ContainerNodeData } from "./container-node";

interface ContainerConfigPanelProps {
  data: ContainerNodeData;
  onChange: (patch: Partial<ContainerNodeData>) => void;
  onClose: () => void;
}

// ContainerConfigPanel edits a selected loop/parallel container's subflow
// settings. The fields shown depend on loopType / parallelType, matching the
// backend graph.Loop / graph.Parallel options the executor understands.
export function ContainerConfigPanel({ data, onChange, onClose }: ContainerConfigPanelProps) {
  const isLoop = data.kind === "loop";
  return (
    <div className="absolute right-2 top-2 w-64 rounded-md border bg-card p-3 shadow-lg">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold">{isLoop ? "循环设置" : "并行设置"}</span>
        <button onClick={onClose} className="text-xs text-muted-foreground hover:text-foreground">
          关闭
        </button>
      </div>

      <label className="mb-2 block text-xs">
        名称
        <input
          className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
          value={data.label}
          onChange={(e) => onChange({ label: e.target.value })}
        />
      </label>

      {isLoop ? (
        <>
          <label className="mb-2 block text-xs">
            循环类型
            <select
              className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
              value={data.loopType ?? "for"}
              onChange={(e) => onChange({ loopType: e.target.value as ContainerNodeData["loopType"] })}
            >
              <option value="for">for（固定次数）</option>
              <option value="forEach">forEach（遍历集合）</option>
              <option value="while">while（前置条件）</option>
              <option value="doWhile">doWhile（后置条件）</option>
            </select>
          </label>

          {(data.loopType ?? "for") === "for" && (
            <label className="mb-2 block text-xs">
              迭代次数
              <input
                type="number"
                min={1}
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                value={data.iterations ?? 1}
                onChange={(e) => onChange({ iterations: Number(e.target.value) })}
              />
            </label>
          )}
          {data.loopType === "forEach" && (
            <label className="mb-2 block text-xs">
              集合引用
              <input
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                placeholder="<block.items>"
                value={data.forEachItems ?? ""}
                onChange={(e) => onChange({ forEachItems: e.target.value })}
              />
            </label>
          )}
          {data.loopType === "while" && (
            <label className="mb-2 block text-xs">
              while 条件 (JS)
              <input
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                placeholder="context.loop.index < 5"
                value={data.whileCondition ?? ""}
                onChange={(e) => onChange({ whileCondition: e.target.value })}
              />
            </label>
          )}
          {data.loopType === "doWhile" && (
            <label className="mb-2 block text-xs">
              doWhile 条件 (JS)
              <input
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                placeholder="context.loop.index < 5"
                value={data.doWhileCondition ?? ""}
                onChange={(e) => onChange({ doWhileCondition: e.target.value })}
              />
            </label>
          )}
        </>
      ) : (
        <>
          <label className="mb-2 block text-xs">
            并行类型
            <select
              className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
              value={data.parallelType ?? "count"}
              onChange={(e) => onChange({ parallelType: e.target.value as ContainerNodeData["parallelType"] })}
            >
              <option value="count">count（固定分支数）</option>
              <option value="collection">collection（按集合分发）</option>
            </select>
          </label>

          {(data.parallelType ?? "count") === "count" && (
            <label className="mb-2 block text-xs">
              分支数
              <input
                type="number"
                min={1}
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                value={data.count ?? 1}
                onChange={(e) => onChange({ count: Number(e.target.value) })}
              />
            </label>
          )}
          {data.parallelType === "collection" && (
            <label className="mb-2 block text-xs">
              集合引用
              <input
                className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
                placeholder="<block.items>"
                value={data.distribution ?? ""}
                onChange={(e) => onChange({ distribution: e.target.value })}
              />
            </label>
          )}
          <label className="mb-2 block text-xs">
            批次大小（可选）
            <input
              type="number"
              min={0}
              className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
              value={data.batchSize ?? 0}
              onChange={(e) => onChange({ batchSize: Number(e.target.value) })}
            />
          </label>
        </>
      )}
    </div>
  );
}
