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
  const hasGovernanceMetadata = Boolean(
    skill.family ||
      skill.aliases?.length ||
      skill.replaces?.length ||
      skill.regression_prefixes?.length ||
      skill.canonical != null,
  );

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
      await updateSkill(skill.id, { content: oldContent.content });
      const nextVersions = await getSkillVersions(skill.id);
      setVersions(nextVersions);
      setSelectedVersion(nextVersions.current);
      toast.success(`已用 v${selectedVersion} 内容创建新的当前版本`);
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
          {hasGovernanceMetadata && (
            <div className="grid gap-2 rounded-md border bg-muted/30 p-3 text-xs sm:grid-cols-2">
              {skill.family && (
                <div>
                  <span className="text-muted-foreground">Skill 家族：</span>
                  <span className="font-medium">{skill.family}</span>
                </div>
              )}
              {skill.canonical != null && (
                <div>
                  <span className="text-muted-foreground">家族主版本：</span>
                  <Badge variant={skill.canonical ? "outline" : "secondary"}>{skill.canonical ? "是" : "否"}</Badge>
                </div>
              )}
              {skill.aliases && skill.aliases.length > 0 && (
                <div className="sm:col-span-2">
                  <span className="text-muted-foreground">中文/历史别名：</span>
                  <span>{skill.aliases.join("、")}</span>
                </div>
              )}
              {skill.replaces && skill.replaces.length > 0 && (
                <div className="sm:col-span-2">
                  <span className="text-muted-foreground">替代旧 Skill：</span>
                  <span>{skill.replaces.join("、")}</span>
                </div>
              )}
              {skill.regression_prefixes && skill.regression_prefixes.length > 0 && (
                <div className="sm:col-span-2">
                  <span className="text-muted-foreground">回归评分前缀：</span>
                  <span>{skill.regression_prefixes.join("、")}</span>
                </div>
              )}
            </div>
          )}
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
              {versions && versions.versions.length > 1 && (
                <div className="flex flex-wrap items-center gap-2">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-muted-foreground">{t("detail.version")}</span>
                    <Select
                      value={String(selectedVersion ?? versions.current)}
                      onValueChange={(v) => setSelectedVersion(Number(v))}
                    >
                      <SelectTrigger className="w-40 h-8">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {versions.versions.map((v) => (
                          <SelectItem key={v} value={String(v)}>
                            v{v}{v === versions.current ? ` ${t("detail.current")}` : ""}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  {!skill.is_system && selectedVersion != null && selectedVersion !== versions.current && (
                    <Button size="sm" variant="outline" onClick={handleCreateVersionFromSelected} disabled={rollingBack}>
                      用此版本创建新版本
                    </Button>
                  )}
                  {skill.is_system && selectedVersion != null && selectedVersion !== versions.current && (
                    <span className="text-xs text-muted-foreground">核心 Skill 不允许前端直接回退，只能作为候选人工处理。</span>
                  )}
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
