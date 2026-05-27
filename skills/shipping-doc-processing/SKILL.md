---
name: 船务清单处理
slug: shipping-doc-processing
family: epl-core-workflow
display_name: 船务清单处理
aliases: shipping-doc-processing, 船务清单
regression_prefixes: shipping_doc_golden_, epl_golden_, ci_to_epl_
description: 处理船务清单相关 Excel 文件，负责检查 CI 和 EPL/PL 是否完整，并根据流转单内容索引或订单映射表补全包装信息。用于已经确认属于船务清单的文件，不负责首轮文件类型识别。通常由核心 skill excel类型识别识别出船务清单后移交到这里继续执行。
---

# 船务清单处理

本 skill 只处理已经确认属于船务清单的文件。

## 工作流程

0. 先确定 `baseDir`：`baseDir` 就是当前 `SKILL.md` 所在目录，不要使用其他同名 skill 目录。
   - 如果当前说明文件路径是 `/app/data/skills-store/shipping-doc-processing/3/SKILL.md`，则 `baseDir=/app/data/skills-store/shipping-doc-processing/3`
   - 如果当前说明文件路径是 `/app/data/skills-store/shipping-doc-processing/2/SKILL.md`，则 `baseDir=/app/data/skills-store/shipping-doc-processing/2`
1. 如果用户要求“完成船务清单”“生成完成版”“处理好后发送/返回文件”，必须直接运行本 skill 自带的生成脚本：
   ```bash
   python3 {baseDir}/scripts/generate_shipping_doc.py "<input-file>" "<output-file>"
   ```
   生成脚本会自动读取流转单内容索引、产品包装重量表和模板结构。不要只运行解析脚本后自行创建 Excel。
2. 仅当用户只是询问诊断信息、不要求生成最终文件时，才运行：
   ```bash
   python3 {baseDir}/scripts/resolve_shipping_doc.py "<shipping-file>"
   ```
3. 从结果中读取：
   - `xs_order_no`
   - `shipping_completeness`
   - `content_index`
   - `mapping`
   - `outer_box`
   - `epl_fill_suggestion`
4. 按以下优先级补包装信息：
   - 优先使用流转单内容索引
   - 内容索引未命中时，再使用订单映射表定位原始流转单 workbook 和 sheet
5. EPL/PL 补全规则：
   - `Dimension = dimension_for_epl`
   - `C/N = c_n_for_epl`
   - 箱数 = `outer_box_qty`
6. 需要生成完成版文件时，运行：
   ```bash
   python3 {baseDir}/scripts/generate_shipping_doc.py "<input-file>" "<output-file>"
   ```
7. 脚本成功后，最终回复必须写明生成后的文件完整路径。不要只说“已完成”，也不要只输出表格摘要。

## 强制规则

- 不要自己手写 sqlite 查询替代脚本
- 不要先向用户索要流转单文件，除非脚本明确返回未命中
- 先跑脚本，再决定如何补全
- 需要输出最终文件时，必须通过本 skill 自带脚本生成，不要在 skill 外手工拼表
- 如果 `exec/Bash` 提示路径安全策略拦截，不允许改为手工拼表；必须改用当前 `SKILL.md` 所在的 `/app/data/skills-store/...` 路径重新运行脚本
- 不允许用 `resolve_shipping_doc.py` 返回的 `epl_fill_suggestion` 直接手工生成最终 Excel；它只是诊断信息，最终文件必须由 `generate_shipping_doc.py` 生成
- 不允许把 `104*26.5*13.2` 这类尺寸中的 `*` 删除或改写；尺寸必须保持 `长*宽*高CM*箱数` 格式
- 对同一订单存在多条包装行时，必须使用生成脚本抽取完整主包装区，不能只使用内容索引里的第一条 `outer_box`
- 如果是配件与上一主产品共箱：
  - 配件行的 `C/N / G.W. / Dimension` 可以留空
- 本 skill 不负责首轮 Excel 类型识别
- 同一业务域如果以后出现别名或升级版本，必须继续归入 `family: epl-core-workflow`，不要再新建平行 skill

## 输出要求

- 明确写出订单号
- 明确写出包装信息来源
- 明确写出建议填入的 `C/N`
- 明确写出建议填入的 `Dimension`
- 生成完成版时，必须明确写出输出文件完整路径
- 如果未命中，也要明确说明是索引未命中还是映射未命中
