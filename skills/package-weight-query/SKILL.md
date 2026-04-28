---
name: 产品包装重量查询
slug: package-weight-query
description: 查询产品包装重量表。用于用户询问产品编码、材料码、系统编码、产品型号、规格型号、材料名称、包装重量、单个重量、外箱尺寸、装箱数量或包装类型时。优先使用产品包装重量表.sqlite 精确查询，不要凭记忆回答。
---

# 产品包装重量查询

用于从 `产品包装重量表.sqlite` 精确查询包装重量和包装基础信息。

## 使用方式

1. 从用户问题中提取产品编码、材料码、系统编码、产品型号、规格型号或材料名称关键词。
2. 运行脚本：
   ```bash
   python3 {baseDir}/scripts/query_package_weight.py "F22塑料槽"
   ```
3. 返回多条结果时，优先展示匹配度最高的前 5 条。
4. 回答时给出产品编码、材料名称、外箱尺寸、包装类型、重量字段、重量、单位、计量基准、来源 sheet、可信度。
5. 如果未命中，明确说“产品包装重量表未查询到该关键词”，不要编造。

## 数据库位置

脚本会自动在以下目录查找有效 sqlite：

- `/mnt/source/product-package-weights`
- `D:\数据\产品包装重量表`

优先查询文件名包含 `产品包装重量表` 的 `.sqlite` 文件，并跳过 0 字节或无表文件。

## 主要字段

- `product_code`：产品编码
- `material_code`：材料码
- `system_code`：系统编码
- `product_model`：产品型号
- `spec_model`：规格型号
- `material_name`：材料名称
- `carton_size`：外箱尺寸
- `packaging_type`：包装类型
- `packaging_form`：包装形式
- `weight_field`：重量字段
- `raw_weight`：原始重量
- `unit`：单位
- `measure_basis`：计量基准
- `weight_g_avg`：折算后的平均重量，单位 g
- `source_sheet_name`：来源 sheet
- `source_position`：来源位置
- `confidence`：可信度

## 回答规则

优先使用 `weight_g_avg` 作为可计算重量；同时保留 `raw_weight`、`unit`、`measure_basis`，方便用户核对原表。
