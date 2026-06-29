import { useState } from "react";
import { Button } from "@/components/ui/button";

export interface FormField {
  id: string;
  label: string;
  type: "select" | "text" | "number" | "confirm" | "checkbox";
  options?: string[];
  default?: string | number | boolean;
  required?: boolean;
}

export interface FormDefinition {
  type: "form";
  title: string;
  fields: FormField[];
}

interface WorkflowFormProps {
  form: FormDefinition;
  onSubmit: (values: Record<string, unknown>) => void;
  disabled?: boolean;
}

// WorkflowForm renders a structured form inside the chat for user interaction
// (like label generation confirmation: select label type, template, quantity).
export function WorkflowForm({ form, onSubmit, disabled }: WorkflowFormProps) {
  const [values, setValues] = useState<Record<string, unknown>>(() => {
    const init: Record<string, unknown> = {};
    for (const f of form.fields) {
      if (f.default !== undefined) init[f.id] = f.default;
      else if (f.type === "checkbox") init[f.id] = false;
      else init[f.id] = "";
    }
    return init;
  });

  const setValue = (id: string, val: unknown) => setValues((v) => ({ ...v, [id]: val }));

  const handleSubmit = () => {
    onSubmit(values);
  };

  return (
    <div className="mt-2 rounded-md border bg-card p-3">
      <div className="mb-3 text-sm font-semibold">{form.title}</div>
      <div className="space-y-3">
        {form.fields.map((f) => {
          if (f.type === "confirm") {
            return (
              <Button key={f.id} size="sm" onClick={handleSubmit} disabled={disabled}>
                {f.label || "确认"}
              </Button>
            );
          }
          return (
            <div key={f.id}>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {f.label} {f.required && <span className="text-red-500">*</span>}
              </label>
              {f.type === "select" && f.options ? (
                <select
                  className="w-full rounded border bg-background px-2 py-1.5 text-sm"
                  value={String(values[f.id] || "")}
                  onChange={(e) => setValue(f.id, e.target.value)}
                  disabled={disabled}
                >
                  <option value="">请选择</option>
                  {f.options.map((opt) => (
                    <option key={opt} value={opt}>{opt}</option>
                  ))}
                </select>
              ) : f.type === "number" ? (
                <input
                  type="number"
                  className="w-full rounded border bg-background px-2 py-1.5 text-sm"
                  value={String(values[f.id] || "")}
                  onChange={(e) => setValue(f.id, Number(e.target.value))}
                  disabled={disabled}
                />
              ) : f.type === "checkbox" ? (
                <label className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    checked={Boolean(values[f.id])}
                    onChange={(e) => setValue(f.id, e.target.checked)}
                    disabled={disabled}
                  />
                  {f.label}
                </label>
              ) : (
                <input
                  type="text"
                  className="w-full rounded border bg-background px-2 py-1.5 text-sm"
                  value={String(values[f.id] || "")}
                  onChange={(e) => setValue(f.id, e.target.value)}
                  disabled={disabled}
                />
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

// tryParseForm attempts to extract a form definition from an assistant message.
// Returns null if the message is not a form.
export function tryParseForm(text: string): FormDefinition | null {
  try {
    // Look for JSON block containing type:"form"
    const jsonMatch = text.match(/```json\s*([\s\S]*?)```/) || text.match(/(\{[\s\S]*"type"\s*:\s*"form"[\s\S]*\})/);
    if (!jsonMatch || !jsonMatch[1]) return null;
    const parsed = JSON.parse(jsonMatch[1]);
    if (parsed?.type === "form" && Array.isArray(parsed?.fields)) {
      return parsed as FormDefinition;
    }
  } catch { /* not a form */ }
  return null;
}
