import { useState, useCallback } from "react";
import { Download, FileText, FileCode, Music, Film, File, Printer } from "lucide-react";
import { MarkdownRenderer } from "@/components/shared/markdown-renderer";
import { formatSize, toDownloadUrl } from "@/lib/file-helpers";
import { useMediaUrl } from "@/hooks/use-media-url";
import { useChatImageGallery } from "./chat-image-gallery-context";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { MediaItem } from "@/types/chat";

const GENERATED_FILENAME_RE = /^[0-9a-f-]{8,}\.png$/i;

function formatTimestamp(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}` +
    `-${pad(d.getHours())}${pad(d.getMinutes())}${pad(d.getSeconds())}`
  );
}

function resolveImageDownloadName(item: MediaItem): string {
  const base = item.fileName ?? "image";
  if (item.mimeType === "image/png" && GENERATED_FILENAME_RE.test(base)) {
    return `generated-${formatTimestamp(new Date())}.png`;
  }
  return base;
}

interface MediaGalleryProps {
  items: MediaItem[];
}

function fileIcon(kind: MediaItem["kind"]) {
  switch (kind) {
    case "document": return <FileText className="h-4 w-4 text-muted-foreground" />;
    case "code":     return <FileCode className="h-4 w-4 text-muted-foreground" />;
    case "audio":    return <Music className="h-4 w-4 text-muted-foreground" />;
    case "video":    return <Film className="h-4 w-4 text-muted-foreground" />;
    default:         return <File className="h-4 w-4 text-muted-foreground" />;
  }
}

function isMarkdownExt(name: string): boolean {
  return /\.(md|mdx|markdown)$/i.test(name);
}

function isMediaKind(kind: string): "image" | "audio" | "video" | null {
  if (kind === "image" || kind === "audio" || kind === "video") return kind;
  return null;
}

function isLabelPreview(item: MediaItem): boolean {
  if (item.kind !== "image") return false;
  const name = (item.fileName ?? item.path).toLowerCase();
  const path = item.path.toLowerCase();
  let decodedPath = path;
  try {
    decodedPath = decodeURIComponent(path);
  } catch {
    decodedPath = path;
  }
  return (
    name === "preview.png" ||
    name === "label_preview.png" ||
    path.includes("/标签作业/") ||
    path.includes("%e6%a0%87%e7%ad%be%e4%bd%9c%e4%b8%9a") ||
    decodedPath.includes("/标签作业/")
  );
}

function escapeHtml(value: string): string {
  return value.replace(/[&<>"']/g, (ch) => ({
    "&": "&amp;",
    "<": "&lt;",
    ">": "&gt;",
    '"': "&quot;",
    "'": "&#39;",
  }[ch] ?? ch));
}

function CachedImage({ src, alt, className, loading, onClick }: {
  src: string; alt: string; className?: string; loading?: "lazy" | "eager";
  onClick?: () => void;
}) {
  const cachedSrc = useMediaUrl(src);
  return <img src={cachedSrc} alt={alt} className={className} loading={loading} onClick={onClick} />;
}

export function MediaGallery({ items }: MediaGalleryProps) {
  const { openImage } = useChatImageGallery();
  const [preview, setPreview] = useState<{
    name: string;
    href: string;
    content: string;
    mediaType?: "image" | "audio" | "video";
  } | null>(null);
  const [loading, setLoading] = useState(false);
  const [printState, setPrintState] = useState<Record<string, "idle" | "printing" | "ok" | "error">>({});
  const [printError, setPrintError] = useState<Record<string, string>>({});

  const handleFileClick = useCallback((item: MediaItem) => {
    const media = isMediaKind(item.kind);
    if (media) {
      setPreview({ name: item.fileName ?? "file", href: item.path, content: "", mediaType: media });
      return;
    }
    setLoading(true);
    fetch(item.path)
      .then((res) => {
        if (!res.ok) throw new Error(res.statusText);
        return res.text();
      })
      .then((text) => setPreview({ name: item.fileName ?? "file", href: item.path, content: text }))
      .catch(() => { /* file preview can fail for unavailable binary assets */ })
      .finally(() => setLoading(false));
  }, []);

  const handlePrintLabel = useCallback((item: MediaItem) => {
    setPrintState((prev) => ({ ...prev, [item.path]: "printing" }));
    setPrintError((prev) => ({ ...prev, [item.path]: "" }));
    try {
      const imageUrl = toDownloadUrl(item.path);
      const existing = document.getElementById("goclaw-label-print-root");
      existing?.remove();

      const root = document.createElement("div");
      root.id = "goclaw-label-print-root";
      root.setAttribute("aria-hidden", "true");
      root.innerHTML = `
        <style>
          #goclaw-label-print-root { display: none; }
          @media print {
            @page { margin: 0; }
            body * { visibility: hidden !important; }
            #goclaw-label-print-root,
            #goclaw-label-print-root * { visibility: visible !important; }
            #goclaw-label-print-root {
              display: block !important;
              position: fixed;
              left: 0;
              top: 0;
              width: 100%;
              height: 100%;
              margin: 0;
              padding: 0;
              background: #fff;
            }
            #goclaw-label-print-root img {
              display: block;
              max-width: 100%;
              max-height: 100%;
              margin: 0;
              padding: 0;
              border: 0;
            }
          }
        </style>
        <img alt="${escapeHtml(item.fileName || "label-preview.png")}" src="${escapeHtml(imageUrl)}" />
      `;
      document.body.appendChild(root);
      const img = root.querySelector("img");
      let fallbackTimer: number | undefined;
      const cleanup = () => {
        if (fallbackTimer) {
          window.clearTimeout(fallbackTimer);
        }
        root.remove();
        window.removeEventListener("afterprint", cleanup);
      };
      const printNow = () => {
        window.addEventListener("afterprint", cleanup, { once: true });
        window.print();
        setPrintState((prev) => ({ ...prev, [item.path]: "ok" }));
        fallbackTimer = window.setTimeout(cleanup, 60000);
      };
      if (img instanceof HTMLImageElement && !img.complete) {
        img.onload = printNow;
        img.onerror = () => {
          root.remove();
          setPrintState((prev) => ({ ...prev, [item.path]: "error" }));
          setPrintError((prev) => ({ ...prev, [item.path]: "标签预览图加载失败" }));
        };
      } else {
        window.setTimeout(printNow, 50);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "打印失败";
      setPrintState((prev) => ({ ...prev, [item.path]: "error" }));
      setPrintError((prev) => ({ ...prev, [item.path]: message }));
    }
  }, []);

  if (items.length === 0) return null;

  const images = items.filter((i) => i.kind === "image");
  const files  = items.filter((i) => i.kind !== "image");

  return (
    <div className="space-y-2">
      {images.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
          {images.map((item, i) => (
            <div key={i} className="flex flex-col overflow-hidden rounded-lg border">
              <div className="group relative">
                <button
                  type="button"
                  onClick={() => openImage(item.path)}
                  className="block w-full cursor-pointer"
                >
                  <CachedImage
                    src={item.path}
                    alt={item.fileName ?? ""}
                    className="h-40 w-full object-cover"
                    loading="lazy"
                  />
                </button>
                <div className="pointer-events-none absolute inset-0 bg-gradient-to-t from-black/50 via-transparent to-transparent opacity-0 transition-opacity group-hover:opacity-100" />
                <div className="absolute inset-x-0 bottom-0 flex items-end justify-between px-2 pb-1.5 opacity-0 transition-opacity group-hover:opacity-100">
                  <div className="flex min-w-0 flex-col text-xs text-white drop-shadow-sm">
                    {item.fileName && <span className="truncate">{item.fileName}</span>}
                    {item.size != null && item.size > 0 && (
                      <span className="text-white/70">{formatSize(item.size)}</span>
                    )}
                  </div>
                  <a
                    href={toDownloadUrl(item.path)}
                    download={resolveImageDownloadName(item)}
                    onClick={(e) => e.stopPropagation()}
                    className="shrink-0 rounded-lg bg-white/90 dark:bg-neutral-800/90 p-1.5 text-neutral-700 dark:text-neutral-200 shadow-md ring-1 ring-black/10 dark:ring-white/10 hover:bg-white dark:hover:bg-neutral-700 transition-colors cursor-pointer"
                    title="Download"
                  >
                    <Download className="h-4.5 w-4.5" />
                  </a>
                </div>
              </div>
              {item.prompt && (
                <div
                  className="px-2 py-1 text-xs text-muted-foreground italic line-clamp-2"
                  title={item.prompt}
                >
                  {item.prompt}
                </div>
              )}
              {isLabelPreview(item) && (
                <div className="flex flex-col gap-1 border-t bg-muted/30 px-2 py-2">
                  <button
                    type="button"
                    onClick={() => handlePrintLabel(item)}
                    disabled={printState[item.path] === "printing"}
                    className="inline-flex items-center justify-center gap-1.5 rounded-md border bg-background px-2.5 py-1.5 text-xs font-medium hover:bg-muted disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    <Printer className="h-3.5 w-3.5" />
                    {printState[item.path] === "printing" ? "正在打印..." : "打印"}
                  </button>
                  {printState[item.path] === "ok" && (
                    <div className="text-xs text-green-600">已打开打印</div>
                  )}
                  {printState[item.path] === "error" && (
                    <div className="text-xs text-destructive" title={printError[item.path]}>
                      {printError[item.path] || "打印失败"}
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {files.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {files.map((item, i) => (
            <div key={i} className="flex items-center rounded-md border bg-muted/50 text-sm">
              <button
                type="button"
                onClick={() => handleFileClick(item)}
                className="flex items-center gap-2 px-3 py-1.5 hover:bg-muted cursor-pointer rounded-l-md"
              >
                {fileIcon(item.kind)}
                <span className="max-w-[200px] truncate">{item.fileName ?? "file"}</span>
                {item.size != null && item.size > 0 && (
                  <span className="text-xs text-muted-foreground">{formatSize(item.size)}</span>
                )}
              </button>
              <a
                href={toDownloadUrl(item.path)}
                download={item.fileName ?? "file"}
                className="flex items-center px-2 py-1.5 text-muted-foreground hover:bg-muted cursor-pointer rounded-r-md border-l"
                onClick={(e) => e.stopPropagation()}
              >
                <Download className="h-3.5 w-3.5" />
              </a>
            </div>
          ))}
        </div>
      )}

      {loading && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/50">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground border-t-transparent" />
        </div>
      )}

      <Dialog open={!!preview} onOpenChange={(open) => { if (!open) setPreview(null); }}>
        {preview && (
          <DialogContent className="sm:max-w-4xl max-h-[85vh] flex flex-col">
            <DialogHeader className="flex-row items-center gap-2 pr-10">
              <DialogTitle className="truncate text-base flex-1">{preview.name}</DialogTitle>
              <a
                href={toDownloadUrl(preview.href)}
                download={preview.name}
                className="flex shrink-0 items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs text-muted-foreground hover:bg-muted"
              >
                <Download className="h-3.5 w-3.5" />
                Download
              </a>
            </DialogHeader>
            <div className="min-h-0 flex-1 overflow-y-auto rounded-md border bg-muted/20 p-4">
              {preview.mediaType === "image" ? (
                <img src={preview.href} alt={preview.name} className="max-w-full rounded" />
              ) : preview.mediaType === "audio" ? (
                <audio controls src={preview.href} className="w-full" />
              ) : preview.mediaType === "video" ? (
                <video controls src={preview.href} className="max-w-full rounded" />
              ) : isMarkdownExt(preview.name) ? (
                <MarkdownRenderer content={preview.content} />
              ) : (
                <pre className="whitespace-pre-wrap text-xs font-mono"><code>{preview.content}</code></pre>
              )}
            </div>
          </DialogContent>
        )}
      </Dialog>
    </div>
  );
}
