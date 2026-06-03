package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEstimateOrderSample(t *testing.T) {
	path := os.Getenv("GOCLAW_ESTIMATE_SAMPLE")
	if path == "" {
		t.Skip("GOCLAW_ESTIMATE_SAMPLE not set")
	}
	path = filepath.Clean(path)
	order, err := parseEstimateOrder(UploadedFileInput{
		Path:     path,
		Filename: "销售订单_25100780.xlsx",
		MIMEType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	})
	if err != nil {
		t.Fatalf("parse estimate order: %v", err)
	}
	if order.OrderNo == "" {
		t.Fatal("expected order number")
	}
	if order.Summary.LineCount == 0 {
		t.Fatal("expected parsed order lines")
	}
	rows := buildEstimateResultRows(order)
	if len(rows) == 0 {
		t.Fatal("expected estimate result rows")
	}
}
