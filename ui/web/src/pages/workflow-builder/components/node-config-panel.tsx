import type { BlockNodeData } from "./block-node";
import { useNodeOptions } from "../hooks/use-node-options";

interface NodeConfigPanelProps {
  data: BlockNodeData;
  onChange: (patch: Partial<BlockNodeData>) => void;
  onClose: () => void;
}

// helper: read/write a single param key
function useParam(data: BlockNodeData, onChange: NodeConfigPanelProps["onChange"]) {
  const params = data.params ?? {};
  const get = (k: string): string => {
    const v = params[k];
    return v == null ? "" : typeof v === "string" ? v : JSON.stringify(v);
  };
  const set = (k: string, v: string) => onChange({ params: { ...params, [k]: v } });
  return { get, set };
}

// NodeConfigPanel renders type-specific configuration for a selected block.
// Each node type exposes the fields its backend handler reads (see
// internal/workflow/handlers). Changes are written back into node data so
// toGraph serializes them into config.params / config.tool.
export function NodeConfigPanel({ data, onChange, onClose }: NodeConfigPanelProps) {
  const { get, set } = useParam(data, onChange);
  const { agents, tools } = useNodeOptions();

  return (
    <div className="absolute right-2 top-2 w-72 rounded-md border bg-card p-3 shadow-lg">
      <div className="mb-2 flex items-center justify-between">
        <span className="text-sm font-semibold">节点设置 · {data.type}</span>
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

      {renderTypeFields(data, get, set, onChange, agents, tools)}
    </div>
  );
}

function select(value: string, options: { value: string; label: string }[], onChange: (v: string) => void, placeholder = "请选择") {
  return (
    <select
      className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">{placeholder}</option>
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </select>
  );
}

function field(label: string, node: React.ReactNode) {
  return (
    <label className="mb-2 block text-xs">
      {label}
      {node}
    </label>
  );
}

function input(value: string, onChange: (v: string) => void, placeholder?: string) {
  return (
    <input
      className="mt-1 w-full rounded border bg-background px-2 py-1 text-sm"
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

function textarea(value: string, onChange: (v: string) => void, placeholder?: string, rows = 4) {
  return (
    <textarea
      className="mt-1 w-full rounded border bg-background px-2 py-1 font-mono text-xs"
      rows={rows}
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

function renderTypeFields(
  data: BlockNodeData,
  get: (k: string) => string,
  set: (k: string, v: string) => void,
  onChange: NodeConfigPanelProps["onChange"],
  agents: { id: string; label: string }[],
  tools: { name: string; label: string }[],
): React.ReactNode {
  switch (data.type) {
    case "agent":
      return (
        <>
          {field(
            "智能体",
            select(
              get("agent"),
              agents.map((a) => ({ value: a.id, label: a.label })),
              (v) => set("agent", v),
              agents.length ? "选择智能体" : "(无可用智能体)",
            ),
          )}
          {field("提示词", textarea(get("prompt"), (v) => set("prompt", v), "给智能体的指令,可用 <trigger.message>"))}
          {field("系统提示(可选)", textarea(get("systemPrompt"), (v) => set("systemPrompt", v), "", 2))}
        </>
      );
    case "tool":
      return (
        <>
          {field(
            "工具",
            select(
              data.tool ?? "",
              tools.map((t) => ({ value: t.name, label: t.label })),
              (v) => onChange({ tool: v }),
              tools.length ? "选择工具" : "(无可用工具)",
            ),
          )}
          {field("参数 (JSON)", textarea(get("__args"), (v) => set("__args", v), '{"query":"..."}'))}
          <p className="text-[10px] text-muted-foreground">参数按 JSON 填写,键为工具入参名。</p>
        </>
      );
    case "condition":
      return (
        <>
          {field("条件表达式 (JS)", textarea(get("expr"), (v) => set("expr", v), "context.score > 0.5", 2))}
          <p className="text-[10px] text-muted-foreground">
            为真走 condition-c1 出边,为假走其它分支。表达式可读 context.&lt;上游字段&gt;。
          </p>
        </>
      );
    case "router":
      return (
        <>
          {field("路由 ID(直选)", input(get("route"), (v) => set("route", v), "目标 router-<id>"))}
          <p className="text-[10px] text-muted-foreground">填写要走的路由 id,对应出边 router-&lt;id&gt;。</p>
        </>
      );
    case "function":
      return field("JavaScript 代码", textarea(get("code"), (v) => set("code", v), "return { result: context.x + 1 }", 6));
    case "api":
      return (
        <>
          {field("URL", input(get("url"), (v) => set("url", v), "https://api.example.com/..."))}
          {field("方法", input(get("method"), (v) => set("method", v), "GET / POST"))}
          {field("请求体 (JSON,可选)", textarea(get("body"), (v) => set("body", v), "", 3))}
        </>
      );
    case "knowledge":
      return (
        <>
          {field("检索词", input(get("query"), (v) => set("query", v), "可用 <trigger.message>"))}
          {field("返回条数", input(get("maxResults"), (v) => set("maxResults", v), "5"))}
        </>
      );
    case "human-in-the-loop":
      return (
        <>
          {field("提示用户", textarea(get("prompt"), (v) => set("prompt", v), "请审核以下内容…", 2))}
          {field("需要填写的字段(逗号分隔)", input(get("fields"), (v) => set("fields", v), "approval,note"))}
        </>
      );
    case "response":
      return field("输出内容", textarea(get("content"), (v) => set("content", v), "可引用 <节点.字段>", 3));
    case "trigger":
      return <p className="text-xs text-muted-foreground">流程入口,无需额外配置。运行时的输入即此节点输出。</p>;
    default:
      return <p className="text-xs text-muted-foreground">该节点类型暂无专属配置。</p>;
  }
}
