package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/agent"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

type evolutionRegressionCase struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type evolutionRegressionRun struct {
	RunID        string                    `json:"run_id"`
	Status       string                    `json:"status"`
	Scope        string                    `json:"scope"`
	SuggestionID string                    `json:"suggestion_id,omitempty"`
	Total        int                       `json:"total"`
	Passed       int                       `json:"passed"`
	Failed       int                       `json:"failed"`
	StartedAt    time.Time                 `json:"started_at"`
	CompletedAt  time.Time                 `json:"completed_at"`
	Cases        []evolutionRegressionCase `json:"cases"`
}

type evolutionAuditEvent struct {
	Action       string    `json:"action"`
	Actor        string    `json:"actor,omitempty"`
	SuggestionID string    `json:"suggestion_id,omitempty"`
	Status       string    `json:"status,omitempty"`
	Result       string    `json:"result"`
	Message      string    `json:"message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (h *EvolutionHandler) handleListRegressionRuns(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	limit := boundedLimit(r.URL.Query().Get("limit"), 20, 100)
	since := time.Now().AddDate(0, 0, -30)
	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricRegression, since, limit)
	if err != nil {
		slog.Warn("evolution.regression.list_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	runs := make([]evolutionRegressionRun, 0, len(metrics))
	for _, metric := range metrics {
		var run evolutionRegressionRun
		if err := json.Unmarshal(metric.Value, &run); err != nil {
			continue
		}
		runs = append(runs, run)
	}
	writeJSON(w, http.StatusOK, runs)
}

func (h *EvolutionHandler) handleRunRegression(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	var body struct {
		Scope        string `json:"scope"`
		SuggestionID string `json:"suggestion_id"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !bindJSON(w, r, locale, &body) {
			return
		}
	}
	if body.Scope == "" {
		body.Scope = "agent_safety"
	}
	if !isValidRegressionScope(body.Scope) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope must be agent_safety, core_skill_smoke, business_workflow_smoke, or business_output_golden"})
		return
	}

	run := h.executeRegressionRun(r, agentID, body.Scope, body.SuggestionID)
	if err := h.recordRegressionRun(r, agentID, run); err != nil {
		slog.Warn("evolution.regression.record_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.recordAuditEvent(r, agentID, "regression_run", body.SuggestionID, run.Status, "ok",
		fmt.Sprintf("%s: %d/%d passed", run.Scope, run.Passed, run.Total))
	writeJSON(w, http.StatusCreated, run)
}

func (h *EvolutionHandler) executeRegressionRun(r *http.Request, agentID uuid.UUID, scope, suggestionID string) evolutionRegressionRun {
	started := time.Now().UTC()
	run := evolutionRegressionRun{
		RunID:        uuid.New().String(),
		Scope:        scope,
		SuggestionID: suggestionID,
		StartedAt:    started,
		Cases:        []evolutionRegressionCase{},
	}
	addCase := func(name, status, message string) {
		run.Cases = append(run.Cases, evolutionRegressionCase{Name: name, Status: status, Message: message})
		if status == "passed" {
			run.Passed++
		} else if status == "failed" {
			run.Failed++
		}
	}

	if h.agentStore == nil {
		addCase("agent_load", "failed", "agent store is not configured")
	} else if ag, err := h.agentStore.GetByID(r.Context(), agentID); err != nil {
		addCase("agent_load", "failed", err.Error())
	} else {
		addCase("agent_load", "passed", fmt.Sprintf("agent %s is readable", ag.AgentKey))
	}

	if items, err := h.suggestions.ListSuggestions(r.Context(), agentID, "", 100); err != nil {
		addCase("suggestions_read", "failed", err.Error())
	} else {
		addCase("suggestions_read", "passed", fmt.Sprintf("%d suggestions readable", len(items)))
		appliedThresholdsWithoutBaseline := 0
		for _, item := range items {
			if item.Status != "applied" || item.SuggestionType != store.SuggestThreshold {
				continue
			}
			var params map[string]any
			_ = json.Unmarshal(item.Parameters, &params)
			if _, ok := params["_baseline"].(map[string]any); !ok {
				appliedThresholdsWithoutBaseline++
			}
		}
		if appliedThresholdsWithoutBaseline > 0 {
			addCase("rollback_baseline", "failed", fmt.Sprintf("%d applied threshold suggestions have no rollback baseline", appliedThresholdsWithoutBaseline))
		} else {
			addCase("rollback_baseline", "passed", "applied threshold suggestions have rollback baselines")
		}
	}

	since := time.Now().AddDate(0, 0, -30)
	if _, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricFeedback, since, 10); err != nil {
		addCase("feedback_metrics_read", "failed", err.Error())
	} else {
		addCase("feedback_metrics_read", "passed", "feedback metrics query is available")
	}
	if _, err := h.metrics.AggregateToolMetrics(r.Context(), agentID, since); err != nil {
		addCase("tool_metrics_aggregate", "failed", err.Error())
	} else {
		addCase("tool_metrics_aggregate", "passed", "tool metrics aggregation is available")
	}
	if _, err := h.metrics.AggregateRetrievalMetrics(r.Context(), agentID, since); err != nil {
		addCase("retrieval_metrics_aggregate", "failed", err.Error())
	} else {
		addCase("retrieval_metrics_aggregate", "passed", "retrieval metrics aggregation is available")
	}

	switch scope {
	case "core_skill_smoke":
		h.addCoreSkillSmokeCases(r, addCase)
	case "business_workflow_smoke":
		h.addCoreSkillSmokeCases(r, addCase)
		h.addBusinessWorkflowSmokeCases(addCase)
	case "business_output_golden":
		h.addCoreSkillSmokeCases(r, addCase)
		h.addBusinessWorkflowSmokeCases(addCase)
		h.addBusinessOutputGoldenCases(r, addCase)
	}

	run.Total = len(run.Cases)
	run.Status = "passed"
	if run.Failed > 0 {
		run.Status = "failed"
	}
	run.CompletedAt = time.Now().UTC()
	return run
}

func isValidRegressionScope(scope string) bool {
	switch scope {
	case "agent_safety", "core_skill_smoke", "business_workflow_smoke", "business_output_golden":
		return true
	default:
		return false
	}
}

func (h *EvolutionHandler) recordRegressionRun(r *http.Request, agentID uuid.UUID, run evolutionRegressionRun) error {
	value, _ := json.Marshal(run)
	return h.metrics.RecordMetric(r.Context(), store.EvolutionMetric{
		ID:         uuid.New(),
		AgentID:    agentID,
		SessionKey: "evolution-center",
		MetricType: store.MetricRegression,
		MetricKey:  run.Status,
		Value:      value,
	})
}

func (h *EvolutionHandler) addCoreSkillSmokeCases(r *http.Request, addCase func(name, status, message string)) {
	if h.skillReader == nil {
		addCase("core_skill_reader", "failed", "skill store is not configured")
		return
	}
	all := h.skillReader.ListSkills(r.Context())
	if len(all) == 0 {
		addCase("core_skill_catalog", "failed", "no skills available for smoke regression")
		return
	}
	seen := make(map[string]struct{})
	for _, info := range all {
		slug := strings.TrimSpace(info.Slug)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		if !shouldIncludeInCoreSkillSmoke(info) {
			continue
		}
		h.addSkillFileSmokeCase(r, slug, addCase)
	}
}

func shouldIncludeInCoreSkillSmoke(info store.SkillInfo) bool {
	if info.Status != "active" && info.Status != "archived" {
		return false
	}
	if strings.TrimSpace(info.Path) == "" {
		return false
	}
	if strings.TrimSpace(info.Slug) == "" {
		return false
	}
	if !info.IsSystem && strings.TrimSpace(info.Family) == "" {
		return false
	}
	return true
}

func (h *EvolutionHandler) addSkillFileSmokeCase(r *http.Request, slug string, addCase func(name, status, message string)) {
	info, ok := h.skillReader.GetSkill(r.Context(), slug)
	if !ok || info == nil {
		addCase("core_skill_"+slug, "failed", "skill is not active or not visible in this tenant")
		return
	}
	if info.Path == "" {
		addCase("core_skill_"+slug, "failed", fmt.Sprintf("skill %s has empty file path", slug))
		return
	}
	stat, err := os.Stat(info.Path)
	if err != nil {
		addCase("core_skill_"+slug, "failed", fmt.Sprintf("SKILL.md not readable: %v", err))
		return
	}
	if stat.Size() == 0 {
		addCase("core_skill_"+slug, "failed", "SKILL.md is empty")
		return
	}
	addCase("core_skill_"+slug, "passed", fmt.Sprintf("%s v%d is readable", info.Name, info.Version))
}

func (h *EvolutionHandler) addBusinessWorkflowSmokeCases(addCase func(name, status, message string)) {
	requiredFiles := []struct {
		name string
		path string
	}{
		{"flow_order_mapping_sqlite", "/mnt/target/flow-orders/全部年份订单映射表.sqlite"},
		{"flow_order_content_sqlite", "/mnt/target/flow-orders/全部年份流转单内容索引.sqlite"},
		{"package_weight_sqlite", "/mnt/source/product-package-weights/产品包装重量表.sqlite"},
		{"package_materials_sqlite", "/mnt/package-materials/包装资料.sqlite"},
	}
	for _, item := range requiredFiles {
		h.addFileReadableCase(item.name, item.path, addCase)
	}

	requiredDirs := []struct {
		name string
		path string
	}{
		{"label_templates_dir", "/mnt/label-templates"},
		{"workspace_storage_dir", "/app/workspace"},
	}
	for _, item := range requiredDirs {
		info, err := os.Stat(item.path)
		if err != nil {
			addCase(item.name, "failed", fmt.Sprintf("%s not readable: %v", item.path, err))
			continue
		}
		if !info.IsDir() {
			addCase(item.name, "failed", fmt.Sprintf("%s is not a directory", item.path))
			continue
		}
		entries, err := os.ReadDir(item.path)
		if err != nil {
			addCase(item.name, "failed", fmt.Sprintf("%s cannot be listed: %v", item.path, err))
			continue
		}
		addCase(item.name, "passed", fmt.Sprintf("%s is readable with %d entries", item.path, len(entries)))
	}

	for _, rel := range []string{"工字标", "唛头", "品名标"} {
		h.addDirectoryReadableCase("label_template_"+rel, filepath.Join("/mnt/label-templates", rel), addCase)
	}
}

type shippingGoldenRegressionOutput struct {
	OK        bool                           `json:"ok"`
	Error     string                         `json:"error"`
	CasesDir  string                         `json:"cases_dir"`
	OutputDir string                         `json:"output_dir"`
	Total     int                            `json:"total"`
	Passed    int                            `json:"passed"`
	Failed    int                            `json:"failed"`
	Cases     []shippingGoldenRegressionCase `json:"cases"`
}

type shippingGoldenRegressionCase struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Score         int      `json:"score"`
	MinScore      int      `json:"min_score"`
	GeneratedFile string   `json:"generated_file"`
	Failures      []string `json:"failures"`
}

func (h *EvolutionHandler) addBusinessOutputGoldenCases(r *http.Request, addCase func(name, status, message string)) {
	scriptPath := filepath.Join("/app/bundled-skills", "shipping-doc-processing", "scripts", "shipping_golden_regression.py")
	if _, err := os.Stat(scriptPath); err != nil {
		fallback := filepath.Join("skills", "shipping-doc-processing", "scripts", "shipping_golden_regression.py")
		if _, fallbackErr := os.Stat(fallback); fallbackErr == nil {
			scriptPath = fallback
		} else {
			addCase("shipping_doc_golden_script", "failed", fmt.Sprintf("%s not readable: %v", scriptPath, err))
			return
		}
	}

	casesDir := "/mnt/test-data/船务清单"
	if _, err := os.Stat(casesDir); err != nil {
		addCase("shipping_doc_golden_cases", "failed", fmt.Sprintf("%s not readable: %v", casesDir, err))
		return
	}

	outputDir := filepath.Join("/app/workspace", "system", "evolution-regression", "shipping-doc-processing", time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		addCase("shipping_doc_golden_output_dir", "failed", fmt.Sprintf("%s cannot be created: %v", outputDir, err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(
		ctx,
		"python3",
		scriptPath,
		"--cases-dir",
		casesDir,
		"--output-dir",
		outputDir,
		"--case",
		"测试订单7",
		"--case",
		"测试订单8",
		"--min-score",
		"75",
	)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		addCase("shipping_doc_golden_timeout", "failed", ctx.Err().Error())
		return
	}
	if stderr.Len() > 0 {
		slog.Warn("evolution.shipping_golden.stderr", "stderr", strings.TrimSpace(stderr.String()))
	}

	var payload shippingGoldenRegressionOutput
	if jsonErr := json.Unmarshal(stdout.Bytes(), &payload); jsonErr != nil {
		message := strings.TrimSpace(stdout.String())
		if len(message) > 500 {
			message = message[:500] + "..."
		}
		addCase("shipping_doc_golden_parse", "failed", fmt.Sprintf("cannot parse regression output: %v; stdout=%s", jsonErr, message))
		return
	}
	if err != nil && payload.Total == 0 {
		addCase("shipping_doc_golden_run", "failed", fmt.Sprintf("script failed: %v; error=%s", err, payload.Error))
		return
	}
	for _, item := range payload.Cases {
		status := "failed"
		if item.Status == "passed" {
			status = "passed"
		}
		message := fmt.Sprintf("score %d/%d, output=%s", item.Score, item.MinScore, item.GeneratedFile)
		if len(item.Failures) > 0 {
			message = fmt.Sprintf("%s, failures=%s", message, strings.Join(item.Failures, "; "))
		}
		addCase("shipping_doc_golden_"+item.Name, status, message)
	}
	if len(payload.Cases) == 0 {
		addCase("shipping_doc_golden_cases", "failed", fmt.Sprintf("no cases executed from %s", payload.CasesDir))
	}
}

func (h *EvolutionHandler) addFileReadableCase(name, path string, addCase func(name, status, message string)) {
	info, err := os.Stat(path)
	if err != nil {
		addCase(name, "failed", fmt.Sprintf("%s not readable: %v", path, err))
		return
	}
	if info.IsDir() {
		addCase(name, "failed", fmt.Sprintf("%s is a directory, expected file", path))
		return
	}
	if info.Size() <= 0 {
		addCase(name, "failed", fmt.Sprintf("%s is empty", path))
		return
	}
	addCase(name, "passed", fmt.Sprintf("%s exists, %.1f KB", path, float64(info.Size())/1024))
}

func (h *EvolutionHandler) addDirectoryReadableCase(name, path string, addCase func(name, status, message string)) {
	info, err := os.Stat(path)
	if err != nil {
		addCase(name, "failed", fmt.Sprintf("%s not readable: %v", path, err))
		return
	}
	if !info.IsDir() {
		addCase(name, "failed", fmt.Sprintf("%s is not a directory", path))
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		addCase(name, "failed", fmt.Sprintf("%s cannot be listed: %v", path, err))
		return
	}
	if len(entries) == 0 {
		addCase(name, "failed", fmt.Sprintf("%s has no template files", path))
		return
	}
	addCase(name, "passed", fmt.Sprintf("%s is readable with %d entries", path, len(entries)))
}

func (h *EvolutionHandler) handleListAudit(w http.ResponseWriter, r *http.Request) {
	agentID, ok := h.resolveAgentID(w, r)
	if !ok {
		return
	}
	limit := boundedLimit(r.URL.Query().Get("limit"), 50, 200)
	since := time.Now().AddDate(0, 0, -90)
	metrics, err := h.metrics.QueryMetrics(r.Context(), agentID, store.MetricAudit, since, limit)
	if err != nil {
		slog.Warn("evolution.audit.list_failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	events := make([]evolutionAuditEvent, 0, len(metrics))
	for _, metric := range metrics {
		var event evolutionAuditEvent
		if err := json.Unmarshal(metric.Value, &event); err != nil {
			continue
		}
		if event.CreatedAt.IsZero() {
			event.CreatedAt = metric.CreatedAt
		}
		events = append(events, event)
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *EvolutionHandler) recordAuditEvent(r *http.Request, agentID uuid.UUID, action, suggestionID, status, result, message string) {
	if h == nil || h.metrics == nil {
		return
	}
	actor := store.ActorIDFromContext(r.Context())
	value, _ := json.Marshal(evolutionAuditEvent{
		Action:       action,
		Actor:        actor,
		SuggestionID: suggestionID,
		Status:       status,
		Result:       result,
		Message:      message,
		CreatedAt:    time.Now().UTC(),
	})
	if err := h.metrics.RecordMetric(r.Context(), store.EvolutionMetric{
		ID:         uuid.New(),
		AgentID:    agentID,
		SessionKey: "evolution-center",
		MetricType: store.MetricAudit,
		MetricKey:  action,
		Value:      value,
	}); err != nil {
		slog.Warn("evolution.audit.record_failed", "action", action, "error", err)
	}
}

func (h *EvolutionHandler) rollbackSuggestion(r *http.Request, sg store.EvolutionSuggestion, reviewedBy string) error {
	if sg.Status == "applied" && sg.SuggestionType == store.SuggestThreshold {
		if h.agentStore == nil {
			return fmt.Errorf("threshold rollback is not available")
		}
		if err := agent.RollbackSuggestion(r.Context(), h.agentStore, h.suggestions, sg); err != nil {
			return err
		}
		if reviewedBy != "" && reviewedBy != "auto-adapt" {
			return h.suggestions.UpdateSuggestionStatus(r.Context(), sg.ID, "rolled_back", reviewedBy)
		}
		return nil
	}
	if sg.Status == "applied" && sg.SuggestionType == store.SuggestSkillRepair {
		return h.rollbackSkillRepair(r.Context(), sg, reviewedBy)
	}
	return h.suggestions.UpdateSuggestionStatus(r.Context(), sg.ID, "rolled_back", reviewedBy)
}

func boundedLimit(raw string, fallback, max int) int {
	limit, _ := strconv.Atoi(raw)
	if limit <= 0 {
		limit = fallback
	}
	if limit > max {
		limit = max
	}
	return limit
}
