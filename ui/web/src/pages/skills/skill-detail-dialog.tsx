import { useState, useEffect, useCallback, useMemo } from "react";
import { useTranslation } from "react-i18next";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { MarkdownRenderer } from "@/components/shared/markdown-renderer";
import type { SkillInfo, SkillFile, SkillVersions } from "@/types/skill";
import { buildTree } from "./skill-file-helpers";
import { FileBrowser } from "./skill-file-browser";
import { formatSkillLabel } from "./skill-label";
import { toast } from "@/stores/use-toast-store";

interface SkillDetailDialogProps {
  skill: SkillInfo & { content: string };
  onClose: () => void;
  getSkillVersions: (id: string) => Promise<SkillVersions>;
  getSkillFiles: (id: string, version?: number) => Promise<SkillFile[]>;
  getSkillFileContent: (id: string, path: string, version?: number) => Promise<{ content: string; path: string; size: number }>;
  updateSkill: (id: string, updates: Record<string, unknown>) => Promise<unknown>;
}

export function SkillDetailDialog({
  skill,
  onClose,
  getSkillVersions,
  getSkillFiles,
  getSkillFileContent,
  updateSkill,
}: SkillDetailDialogProps) {
  const { t } = useTranslation("skills");
  const hasFiles = !!skill.id;

  // Version state
  const [versions, setVersions] = useState<SkillVersions | null>(null);
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null);

  // File tree state
  const [files, setFiles] = useState<SkillFile[]>([]);
  const [filesLoading, setFilesLoading] = useState(false);
  const [activePath, setActivePath] = useState<string | null>(null);

  // File content state
  const [fileContent, setFileContent] = useState<{ content: string; path: string; size: number } | null>(null);
  const [contentLoading, setContentLoading] = useState(false);
  const [rollingBack, setRollingBack] = useState(false);

  const tree = useMemo(() => buildTree(files), [files]);
  const effectiveFamily = skill.family || skill.slug || skill.name;
  const selectedIsCurrent = versions != null && selectedVersion === versions.current;
  const selectedVersionLabel = selectedVersion == null ? "未选择" : `v${selectedVersion}`;

  const loadVersions = useCallback(async () => {
    if (!skill.id || versions) return;
    const v = await getSkillVersions(skill.id);
    setVersions(v);
    setSelectedVersion(v.current);
  }, [skill.id, versions, getSkillVersions]);

  const loadFiles = useCallback(async (version?: number) => {
    if (!skill.id) return;
    setFilesLoading(true);
    try {
      const f = await getSkillFiles(skill.id, version);
      setFiles(f);
      setActivePath(null);
      setFileContent(null);
    } finally {
      setFilesLoading(false);
    }
  }, [skill.id, getSkillFiles]);

  const loadFileContent = useCallback(async (path: string) => {
    if (!skill.id) return;
    setActivePath(path);
    setContentLoading(true);
    try {
      const c = await getSkillFileContent(skill.id, path, selectedVersion ?? undefined);
      setFileContent(c);
    } finally {
      setContentLoading(false);
    }
  }, [skill.id, selectedVersion, getSkillFileContent]);

  useEffect(() => {
    if (selectedVersion != null) {
      loadFiles(selectedVersion);
    }
  }, [selectedVersion, loadFiles]);

  useEffect(() => {
    if (hasFiles) {
      loadVersions();
    }
  }, [hasFiles, loadVersions]);

  const handleTabChange = (tab: string) => {
    if (tab === "files" && hasFiles) {
      loadVersions();
      if (files.length === 0 && !filesLoading) {
        loadFiles(selectedVersion ?? undefined);
      }
    }
  };

  const handleCreateVersionFromSelected = async () => {
    if (!skill.id || !versions || selectedVersion == null || selectedVersion === versions.current || skill.is_system) return;
    setRollingBack(true);
    try {
      const oldContent = await getSkillFileContent(skill.id, "SKILL.md", selectedVersion);
      const restoredVersion = selectedVersion;
      await updateSkill(skill.id, { content: oldContent.content });
      const nextVersions = await getSkillVersions(skill.id);
      setVersions(nextVersions);
      setSelectedVersion(nextVersions.current);
      toast.success(`已按指定版本恢复：已用 v${restoredVersion} 内容创建新的当前版本 v${nextVersions.current}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : "未知错误";
      toast.error("版本回退失败", message);
    } finally {
      setRollingBack(false);
    }
  };

  return (
    <Dialog open onOpenChange={() => onClose()}>
      <DialogContent className="max-h-[85vh] md:min-h-[60vh] overflow-hidden flex flex-col sm:max-w-2xl md:max-w-4xl lg:max-w-5xl xl:max-w-6xl 2xl:max-w-7xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 flex-wrap">
            {formatSkillLabel(skill)}
            <Badge variant="outline">{skill.source || "file"}</Badge>
            {skill.visibility && (
              <Badge variant="secondary">{skill.visibility}</Badge>
            )}
          </DialogTitle>
          {skill.description && (
            <p className="text-sm text-muted-foreground">{skill.description}</p>
          )}
          {skill.tags && skill.tags.length > 0 && (
            <div className="flex flex-wrap gap-1 pt-1">
              {skill.tags.map((tag) => (
                <Badge key={tag} variant="outline" className="text-xs">{tag}</Badge>
              ))}
            </div>
          )}
          <div className="grid gap-3 rounded-md border bg-muted/30 p-3 text-xs md:grid-cols-2">
            <div className="space-y-2">
              <p className="text-sm font-medium">Skill 家族治理</p>
              <div>
                <span className="text-muted-foreground">所属家族：</span>
                <span className="font-medium">{effectiveFamily}</span>
                {!skill.family && <span className="ml-1 text-muted-foreground">（未显式配置，默认按英文名称归属）</span>}
              </div>
              <div>
                <span className="text-muted-foreground">是否家族主版本：</span>
                {skill.canonical == null ? (
                  <span>未显式配置，默认作为本家族可用版本</span>
                ) : (
                  <Badge variant={skill.canonical ? "outline" : "secondary"}>{skill.canonical ? "是" : "否"}</Badge>
                )}
              </div>
              <div>
                <span className="text-muted-foreground">中文/历史别名：</span>
                <span>{skill.aliases && skill.aliases.length > 0 ? skill.aliases.join("、") : "未配置"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">替代旧 Skill：</span>
                <span>{skill.replaces && skill.replaces.length > 0 ? skill.replaces.join("、") : "未配置"}</span>
              </div>
              <div>
                <span className="text-muted-foreground">回归评分前缀：</span>
                <span>{skill.regression_prefixes && skill.regression_prefixes.length > 0 ? skill.regression_prefixes.join("、") : "未配置"}</span>
              </div>
            </div>

            <div className="space-y-2">
              <p className="text-sm font-medium">版本管理</p>
              <div>
                <span className="text-muted-foreground">当前线上版本：</span>
                <span className="font-medium">{versions ? `v${versions.current}` : skill.version ? `v${skill.version}` : "加载中"}</span>
              </div>
              {versions && versions.versions.length > 0 ? (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">选择要查看/恢复的版本：</span>
                  <Select
                    value={String(selectedVersion ?? versions.current)}
                    onValueChange={(v) => setSelectedVersion(Number(v))}
                  >
                    <SelectTrigger className="h-8 w-44">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {versions.versions.map((v) => (
                        <SelectItem key={v} value={String(v)}>
                          v{v}{v === versions.current ? "（当前线上）" : ""}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              ) : (
                <p className="text-muted-foreground">正在加载版本列表...</p>
              )}
              <div className="flex flex-wrap items-center gap-2">
                {!skill.is_system && versions && selectedVersion != null && !selectedIsCurrent && (
                  <Button size="sm" variant="outline" onClick={handleCreateVersionFromSelected} disabled={rollingBack}>
                    恢复到选中的 {selectedVersionLabel}
                  </Button>
                )}
                {!skill.is_system && versions && selectedIsCurrent && (
                  <span className="text-muted-foreground">当前选择的是线上版本，不需要回退。</span>
                )}
                {skill.is_system && (
                  <span className="text-muted-foreground">核心 Skill 不允许前端直接回退，只能生成候选后人工处理。</span>
                )}
              </div>
              {versions && selectedVersion != null && !selectedIsCurrent && !skill.is_system && (
                <p className="text-muted-foreground">
                  操作说明：不会覆盖历史 v{selectedVersion}，会把 v{selectedVersion} 的内容复制成新的当前版本，方便后续继续回滚。
                </p>
              )}
            </div>
          </div>
        </DialogHeader>

        <Tabs defaultValue="content" className="flex-1 overflow-hidden flex flex-col" onValueChange={handleTabChange}>
          <TabsList>
            <TabsTrigger value="content">{t("detail.content")}</TabsTrigger>
            {hasFiles && <TabsTrigger value="files">{t("detail.files")}</TabsTrigger>}
          </TabsList>

          <TabsContent value="content" className="flex-1 overflow-y-auto mt-2 -mx-4 px-4 sm:-mx-6 sm:px-6">
            {skill.content ? (
              <div className="overflow-hidden rounded-md border bg-muted/30 p-4">
                <MarkdownRenderer content={skill.content} />
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t("detail.noContent")}</p>
            )}
          </TabsContent>

          {hasFiles && (
            <TabsContent value="files" className="flex-1 overflow-hidden flex flex-col mt-2 gap-2">
              {versions && selectedVersion != null && (
                <div className="rounded-md border bg-muted/30 px-3 py-2 text-sm">
                  正在查看：{selectedVersionLabel}
                  {selectedIsCurrent ? "（当前线上版本）" : "（历史版本）"}
                </div>
              )}

              <FileBrowser
                tree={tree}
                filesLoading={filesLoading}
                activePath={activePath}
                onSelect={loadFileContent}
                contentLoading={contentLoading}
                fileContent={fileContent}
              />
            </TabsContent>
          )}
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
