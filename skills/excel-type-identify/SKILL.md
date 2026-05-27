---
name: excel类型识别
slug: excel-type-identify
family: excel-type-identify
display_name: excel类型识别
aliases: excel-type-identify, excel类型识别, Excel 类型识别, 文件类型识别
regression_prefixes: excel_type_, excel_detect_
description: 识别用户上传的 Excel 文件属于什么业务类型，并判断下一步应该交给哪个专门的核心 skill 或处理流程。用于 .xls、.xlsx、.csv 文件的首轮分流，不直接执行复杂业务补全。遇到船务清单时，只负责识别并移交给核心 skill 船务清单处理，不在本 skill 内直接补全 EPL/PL。
---

# excel类型识别

先识别，再分流。不要在类型还不清楚时直接改表、补数据或生成结果文件。

## 工作流程

1. 先确认文件路径、扩展名、文件名关键词。
2. 运行：
   ```bash
   python3 {baseDir}/scripts/detect_excel_type.py "<excel-file>"
   ```
3. 根据脚本结果输出：
   - 文件类型
   - 识别依据
   - 建议下一步
   - 是否需要调用其他核心 skill

## 分流规则

- 如果识别为 `船务清单`：
  - 不要在本 skill 内查询流转单
  - 不要在本 skill 内补 EPL/PL
  - 立即调用核心 skill `船务清单处理`
- 如果识别为 `流转单`：
  - 建议进入流转单查询或索引流程
- 如果识别为 `订单映射表`：
  - 建议进行订单定位、查重、筛选
- 如果识别为 `商业发票/CI` 或 `装箱单/PL/EPL`：
  - 先判断是否属于船务清单上下文
  - 如果文件中带有 `XS` 订单号，且明显是待补全的 CI-only / PL-only 文件，也视为船务清单处理范围
  - 若属于完整或待补全船务清单，移交 `船务清单处理`
- 如果识别结果置信度低：
  - 再人工查看文件名、sheet 名、前 10 行表头

## 输出要求

- 明确写出 `detected_type`
- 明确写出 `confidence`
- 明确写出 `matched_rules`
- 明确写出 `next_actions`
- 如果需要专门 skill，直接指出 skill 名称

## 强制边界

- 不要在本 skill 内直接执行船务清单补全
- 不要在本 skill 内直接查询 `flow_content_index` 或 `order_mapping`
- 不要在本 skill 内生成最终船务清单文件
- 本 skill 的职责仅限于识别类型和决定下一步处理方向
