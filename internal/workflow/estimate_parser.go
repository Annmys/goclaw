package workflow

import (
	"archive/zip"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type EstimateOrder struct {
	OrderNo        string               `json:"order_no"`
	CustomerCode   string               `json:"customer_code,omitempty"`
	FinishDate     string               `json:"finish_date,omitempty"`
	SalesPerson    string               `json:"sales_person,omitempty"`
	Label          string               `json:"label,omitempty"`
	PackingRequest string               `json:"packing_request,omitempty"`
	LabelRequest   string               `json:"label_request,omitempty"`
	ProjectName    string               `json:"project_name,omitempty"`
	Rows           []EstimateOrderLine  `json:"rows"`
	ColumnMap      map[string]string    `json:"column_map"`
	MissingColumns []string             `json:"missing_columns,omitempty"`
	Warnings       []string             `json:"warnings,omitempty"`
	Summary        EstimateOrderSummary `json:"summary"`
	Source         UploadedFileInput    `json:"source"`
}

type EstimateOrderLine struct {
	RowNo         int               `json:"row_no"`
	MaterialCode  string            `json:"material_code,omitempty"`
	SpecModel     string            `json:"spec_model,omitempty"`
	Quantity      float64           `json:"quantity,omitempty"`
	CutLength     string            `json:"cut_length,omitempty"`
	CustomerModel string            `json:"customer_model,omitempty"`
	OutputMode    string            `json:"output_mode,omitempty"`
	Raw           map[string]string `json:"raw,omitempty"`
}

type EstimateOrderSummary struct {
	LineCount        int     `json:"line_count"`
	TotalQuantity    float64 `json:"total_quantity"`
	MainMaterialRows int     `json:"main_material_rows"`
	AccessoryRows    int     `json:"accessory_rows"`
	UnknownRows      int     `json:"unknown_rows"`
}

type EstimateResultRow struct {
	ItemCode   string  `json:"item_code"`
	ItemName   string  `json:"item_name"`
	ItemType   string  `json:"item_type"`
	SpecModel  string  `json:"spec_model,omitempty"`
	Quantity   float64 `json:"quantity"`
	Unit       string  `json:"unit"`
	Source     string  `json:"source"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

type xlsxSST struct {
	Items []xlsxSI `xml:"si"`
}

type xlsxSI struct {
	Text string `xml:"t"`
	Runs []struct {
		Text string `xml:"t"`
	} `xml:"r"`
}

type xlsxWorksheet struct {
	SheetData struct {
		Rows []xlsxRow `xml:"row"`
	} `xml:"sheetData"`
}

type xlsxRow struct {
	Index int        `xml:"r,attr"`
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref       string `xml:"r,attr"`
	Type      string `xml:"t,attr"`
	Value     string `xml:"v"`
	InlineStr struct {
		Text string `xml:"t"`
	} `xml:"is"`
}

func parseEstimateOrder(file UploadedFileInput) (EstimateOrder, error) {
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(file.Path))
	}
	var rows [][]string
	var err error
	switch ext {
	case ".xlsx":
		rows, err = readXLSXRows(file.Path)
	case ".csv":
		rows, err = readCSVRows(file.Path)
	default:
		return EstimateOrder{}, fmt.Errorf("unsupported estimate order file type: %s", ext)
	}
	if err != nil {
		return EstimateOrder{}, err
	}
	return buildEstimateOrder(file, rows)
}

func readCSVRows(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}

func readXLSXRows(path string) ([][]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	shared, err := readSharedStrings(&zr.Reader)
	if err != nil {
		return nil, err
	}
	sheetName, err := firstWorksheetName(&zr.Reader)
	if err != nil {
		return nil, err
	}
	rc, err := openZipFile(&zr.Reader, sheetName)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var ws xlsxWorksheet
	if err := xml.NewDecoder(rc).Decode(&ws); err != nil {
		return nil, err
	}

	out := make([][]string, 0, len(ws.SheetData.Rows))
	for _, row := range ws.SheetData.Rows {
		values := make([]string, 0, len(row.Cells))
		maxCol := 0
		cellValues := map[int]string{}
		for _, cell := range row.Cells {
			col := columnIndex(cell.Ref)
			if col <= 0 {
				col = len(values) + 1
			}
			if col > maxCol {
				maxCol = col
			}
			cellValues[col] = resolveXLSXCell(cell, shared)
		}
		for col := 1; col <= maxCol; col++ {
			values = append(values, cellValues[col])
		}
		out = append(out, values)
	}
	return out, nil
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	rc, err := openZipFile(zr, "xl/sharedStrings.xml")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer rc.Close()
	var sst xlsxSST
	if err := xml.NewDecoder(rc).Decode(&sst); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sst.Items))
	for _, item := range sst.Items {
		if item.Text != "" {
			out = append(out, item.Text)
			continue
		}
		var b strings.Builder
		for _, run := range item.Runs {
			b.WriteString(run.Text)
		}
		out = append(out, b.String())
	}
	return out, nil
}

func firstWorksheetName(zr *zip.Reader) (string, error) {
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/sheet") && strings.HasSuffix(f.Name, ".xml") {
			return f.Name, nil
		}
	}
	return "", os.ErrNotExist
}

func openZipFile(zr *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, os.ErrNotExist
}

func resolveXLSXCell(cell xlsxCell, shared []string) string {
	switch cell.Type {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err == nil && idx >= 0 && idx < len(shared) {
			return strings.TrimSpace(shared[idx])
		}
	case "inlineStr":
		return strings.TrimSpace(cell.InlineStr.Text)
	}
	return strings.TrimSpace(cell.Value)
}

var cellRefRe = regexp.MustCompile(`^([A-Z]+)`)

func columnIndex(ref string) int {
	m := cellRefRe.FindStringSubmatch(strings.ToUpper(ref))
	if len(m) < 2 {
		return 0
	}
	n := 0
	for _, ch := range m[1] {
		n = n*26 + int(ch-'A'+1)
	}
	return n
}

func buildEstimateOrder(file UploadedFileInput, rows [][]string) (EstimateOrder, error) {
	headerRow := -1
	headers := map[string]int{}
	for i, row := range rows {
		headers = detectEstimateHeaders(row)
		if len(headers) >= 3 {
			headerRow = i
			break
		}
	}
	if headerRow < 0 {
		return EstimateOrder{}, errors.New("未识别到预估箱单订单表头")
	}

	order := EstimateOrder{
		ColumnMap: map[string]string{},
		Source:    file,
	}
	for key, idx := range headers {
		if idx >= 0 && idx < len(rows[headerRow]) {
			order.ColumnMap[key] = rows[headerRow][idx]
		}
	}
	required := []string{"order_no", "spec_model", "quantity", "material_code"}
	for _, key := range required {
		if _, ok := headers[key]; !ok {
			order.MissingColumns = append(order.MissingColumns, key)
		}
	}

	for i := headerRow + 1; i < len(rows); i++ {
		row := rows[i]
		if emptyRow(row) {
			continue
		}
		line := EstimateOrderLine{
			RowNo: i + 1,
			Raw:   map[string]string{},
		}
		line.MaterialCode = cellByKey(row, headers, "material_code")
		line.SpecModel = cellByKey(row, headers, "spec_model")
		line.Quantity = parseFloat(cellByKey(row, headers, "quantity"))
		line.CutLength = cellByKey(row, headers, "cut_length")
		line.CustomerModel = cellByKey(row, headers, "customer_model")
		line.OutputMode = classifyOutputMode(line.MaterialCode)
		for key, idx := range headers {
			if idx >= 0 && idx < len(row) {
				line.Raw[key] = strings.TrimSpace(row[idx])
			}
		}
		if order.OrderNo == "" {
			order.OrderNo = cellByKey(row, headers, "order_no")
		}
		if order.CustomerCode == "" {
			order.CustomerCode = cellByKey(row, headers, "customer_code")
		}
		if order.FinishDate == "" {
			order.FinishDate = cellByKey(row, headers, "finish_date")
		}
		if order.SalesPerson == "" {
			order.SalesPerson = cellByKey(row, headers, "sales_person")
		}
		if order.Label == "" {
			order.Label = cellByKey(row, headers, "label")
		}
		if order.PackingRequest == "" {
			order.PackingRequest = cellByKey(row, headers, "packing_request")
		}
		if order.LabelRequest == "" {
			order.LabelRequest = cellByKey(row, headers, "label_request")
		}
		if order.ProjectName == "" {
			order.ProjectName = cellByKey(row, headers, "project_name")
		}
		if line.MaterialCode == "" && line.SpecModel == "" && line.Quantity == 0 {
			order.Summary.UnknownRows++
			continue
		}
		order.Rows = append(order.Rows, line)
		order.Summary.LineCount++
		order.Summary.TotalQuantity += line.Quantity
		switch line.OutputMode {
		case "main":
			order.Summary.MainMaterialRows++
		case "accessory":
			order.Summary.AccessoryRows++
		default:
			order.Summary.UnknownRows++
		}
	}
	if order.OrderNo == "" {
		order.OrderNo = strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
		order.Warnings = append(order.Warnings, "未从订单列识别到订单号，已使用文件名作为订单号")
	}
	if len(order.MissingColumns) > 0 {
		order.Warnings = append(order.Warnings, "订单缺少关键列: "+strings.Join(order.MissingColumns, ", "))
	}
	if len(order.Rows) == 0 {
		return order, errors.New("预估箱单订单没有可处理明细行")
	}
	return order, nil
}

func detectEstimateHeaders(row []string) map[string]int {
	out := map[string]int{}
	for i, v := range row {
		key := normalizeHeader(v)
		switch key {
		case "单据编号", "订单编号", "订单号":
			out["order_no"] = i
		case "规格型号", "型号", "产品型号":
			out["spec_model"] = i
		case "销售数量", "数量", "订单数量":
			out["quantity"] = i
		case "物料编码", "物料代码", "物料":
			out["material_code"] = i
		case "剪切长度", "长度":
			out["cut_length"] = i
		case "客户代码":
			out["customer_code"] = i
		case "表头要求完工日期", "完工日期":
			out["finish_date"] = i
		case "客户业务员", "业务员":
			out["sales_person"] = i
		case "标签":
			out["label"] = i
		case "包装要求":
			out["packing_request"] = i
		case "客户型号1", "客户型号":
			out["customer_model"] = i
		case "标签要求":
			out["label_request"] = i
		case "工程名称":
			out["project_name"] = i
		}
	}
	return out
}

func normalizeHeader(v string) string {
	return strings.TrimSpace(strings.ReplaceAll(v, " ", ""))
}

func cellByKey(row []string, headers map[string]int, key string) string {
	idx, ok := headers[key]
	if !ok || idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

func emptyRow(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

func parseFloat(v string) float64 {
	v = strings.TrimSpace(strings.ReplaceAll(v, ",", ""))
	if v == "" {
		return 0
	}
	n, _ := strconv.ParseFloat(v, 64)
	return n
}

func classifyOutputMode(materialCode string) string {
	code := strings.TrimSpace(materialCode)
	switch {
	case strings.HasPrefix(code, "80."), strings.HasPrefix(code, "800202"):
		return "main"
	case strings.HasPrefix(code, "30."), strings.HasPrefix(code, "3002"):
		return "accessory"
	default:
		return "unknown"
	}
}

func buildEstimateResultRows(order EstimateOrder) []EstimateResultRow {
	grouped := map[string]*EstimateResultRow{}
	for _, line := range order.Rows {
		itemType := "待确认包材"
		itemCode := "UNKNOWN"
		itemName := "未匹配包材"
		confidence := 0.55
		reason := "未命中物料编码规则，需要包装资料补充"
		switch line.OutputMode {
		case "main":
			itemType = "主产品包装"
			itemCode = "PACK-" + safeKey(line.SpecModel)
			itemName = "预估主产品包装"
			confidence = 0.72
			reason = "物料编码命中 80./800202 主产品行，按规格型号汇总预估"
		case "accessory":
			itemType = "附件/说明书"
			itemCode = "ACC-" + safeKey(line.SpecModel)
			itemName = "预估附件包材"
			confidence = 0.68
			reason = "物料编码命中 30./3002 附件行，按规格型号汇总预估"
		}
		key := itemCode + "|" + itemType
		if existing, ok := grouped[key]; ok {
			existing.Quantity += line.Quantity
			continue
		}
		grouped[key] = &EstimateResultRow{
			ItemCode:   itemCode,
			ItemName:   itemName,
			ItemType:   itemType,
			SpecModel:  line.SpecModel,
			Quantity:   line.Quantity,
			Unit:       "pcs",
			Source:     fmt.Sprintf("订单行 %d", line.RowNo),
			Confidence: confidence,
			Reason:     reason,
		}
	}
	out := make([]EstimateResultRow, 0, len(grouped))
	for _, row := range grouped {
		out = append(out, *row)
	}
	return out
}

func safeKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "UNKNOWN"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", "(", "", ")", "")
	return replacer.Replace(v)
}
