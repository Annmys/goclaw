---
name: 流转单查询
slug: flow-order-query
description: 查询包装流转单订单索引。用于用户询问 XS 订单号对应的流转单位置、sheet、外箱规格、外箱数量、EPL/PL 应填 C/N 或 Dimension，或需要根据订单号补全船务清单包装信息时。优先使用 sqlite 精确查询，不要凭记忆回答。
---

# 流转单查询

用于从 `全部年份流转单内容索引.sqlite` 精确查询订单包装信息。

## 使用方式

1. 从用户问题中提取 `XS` 订单号，例如 `XS25110436`。
2. 必须直接运行现成脚本，不要自己手写 sqlite 查询，不要用 `sqlite3` CLI，不要凭记忆回答。
3. 运行脚本：
   ```bash
   python3 {baseDir}/scripts/query_flow_order.py XS25110436
   ```
   在 Anchor 运行环境中，优先使用：
   ```bash
   python3 /app/workspace/anchor/ws/system/query_flow_order.py XS25110436
   ```
4. 如果 `content_index` 未命中，脚本会自动回退到 `订单映射表 + 原始 workbook + sheet`，不要自己重复实现这套逻辑。
5. 回答时给出订单号、流转单文件、sheet、外箱规格、外箱数量、`C/N`、`Dimension`。
6. 如果脚本明确返回未命中，再说“内容索引未查询到该订单”，不要编造。

## 强制规则

- 不要自己执行 `select ... from flow_content_index`
- 不要自己执行 `select ... from order_mapping`
- 不要尝试 `sqlite3 xxx.sqlite`
- 不要要求用户先上传流转单，除非脚本返回未命中
- 对 `XS26030756` 这类订单，允许脚本通过回退逻辑从原始流转单中读取包装信息

## 数据库位置

脚本会自动在以下目录查找有效 sqlite：

- `/mnt/target/flow-orders`
- `D:\数据\包装流转单`

优先查询文件名包含 `流转单内容索引` 的 `.sqlite` 文件，并跳过 0 字节或无表文件。

## 返回字段

- `order_no`：订单号
- `year_folder`：年份目录
- `owner_folder`：人员目录
- `workbook_name`：流转单工作簿名
- `sheet_name`：sheet 名
- `workbook_path`：容器内流转单路径
- `outer_box_spec`：外箱规格
- `outer_box_qty`：外箱数量
- `dimension_for_epl`：EPL/PL Dimension 建议值
- `c_n_for_epl`：EPL/PL C/N 建议值

## 船务清单规则

处理船务清单时，包装信息以流转单内容索引为准：

- `Dimension = dimension_for_epl`
- `C/N = c_n_for_epl`
- 箱数 = `outer_box_qty`

不要先要求用户上传流转单文件，除非 sqlite 内容索引没有命中该订单。
