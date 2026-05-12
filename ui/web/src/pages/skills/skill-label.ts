import type { SkillInfo } from "@/types/skill";

export function formatSkillLabel(skill: Pick<SkillInfo, "name" | "slug" | "version">): string {
  const slug = skill.slug?.trim();
  const name = skill.name?.trim();
  const base = slug && name && slug !== name ? `${slug}（${name}）` : (slug || name || "");
  return skill.version ? `${base}V${skill.version}` : base;
}
