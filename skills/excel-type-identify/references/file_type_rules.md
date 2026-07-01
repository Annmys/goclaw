# Excel 类型识别规则

## 船务清单

- 同时存在 `CI` 和 `PL` / `EPL` sheet
- 或文件内容同时出现 `COMMERCIAL INVOICE` 与 `PACKING LIST`
- 识别到 `XS` 订单号时，说明后续通常要进入船务清单补全过程
- 本 skill 只做识别和分流，后续交给 `epl-core-workflow（预估箱单制作核心流程）V2`

## 流转单

- 常见关键词：
  - `流转单`
  - `包装材料需求流转单`
  - `订单号`
  - `外箱`

## 订单映射表

- 常见关键词：
  - `订单映射`
  - `年份目录`
  - `workbook_path`
  - `sheet_name`

## 商业发票 / CI

- 常见关键词：
  - `COMMERCIAL INVOICE`
  - `Invoice`
  - `Amount`
  - `USD`

## 装箱单 / PL / EPL

- 常见关键词：
  - `PACKING LIST`
  - `Estimated Packing List`
  - `C/N`
  - `G.W.(KG)`
  - `Dimension`
