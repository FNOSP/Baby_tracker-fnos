package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type backupTable struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

type backupPayload struct {
	Version    any                    `json:"version"`
	ExportedAt string                 `json:"exported_at"`
	Tables     map[string]backupTable `json:"tables"`
}

var backupColumns = map[string][]string{
	"babies":                    {"id", "name", "gender", "birth_date", "weight", "is_premature", "created_at"},
	"records":                   {"id", "baby_id", "user_id", "type", "sub_type", "amount", "duration", "color", "consistency", "temperature", "note", "timestamp", "food_unit", "created_at"},
	"settings":                  {"id", "key", "value", "updated_at"},
	"users":                     {"id", "username", "password_hash", "nickname", "role", "status", "created_at"},
	"quick_buttons":             {"id", "type", "sub_type", "label", "amount", "sort_order", "is_active", "food_unit", "created_at"},
	"weight_logs":               {"id", "baby_id", "weight", "recorded_date", "note", "created_at"},
	"vaccine_records":           {"id", "vaccine_name", "dose_index", "vaccinated_date", "note", "created_at"},
	"vaccine_plan_overrides":    {"vaccine_name", "dose_index", "custom_due_date"},
	"vaccine_plan_deleted":      {"vaccine_name", "dose_index"},
	"vaccine_custom_plans":      {"vaccine_name", "dose_index", "due_date", "note"},
	"health_followup_records":   {"id", "label", "completed_date", "note", "created_at"},
	"health_followup_overrides": {"label", "custom_due_date"},
	"countdown_events":          {"id", "title", "target_date", "note", "created_at"},
}

var restoreOrder = []string{
	"babies", "users", "settings", "quick_buttons", "records", "weight_logs",
	"vaccine_records", "vaccine_plan_overrides", "vaccine_plan_deleted",
	"vaccine_custom_plans", "health_followup_records", "health_followup_overrides",
	"countdown_events",
}

func (a *App) exportCSV(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(apiUser(r)) {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	rows, err := a.queryMaps(r.Context(), "SELECT * FROM records ORDER BY timestamp DESC, id DESC")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}
	typeMap := map[string]string{
		"feed": "喂养", "excrete": "排泄", "symptom": "症状", "supplement": "补充",
	}
	subTypeMap := map[string]string{
		"breast_left": "母乳(左)", "breast_right": "母乳(右)", "formula": "配方奶", "water": "水",
		"urine": "尿", "stool": "便", "both": "尿+便", "vomit": "呕吐", "fever": "发热",
		"jaundice": "黄疸", "rash": "皮疹", "vitamin_d": "维D", "vitamin_ad": "维AD",
		"iron": "铁剂", "calcium": "钙剂", "dha": "DHA", "probiotics": "益生菌",
	}
	var output bytes.Buffer
	output.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&output)
	_ = writer.Write([]string{"ID", "类型", "子类型", "量(ml)", "时长(分)", "颜色", "性状", "体温", "备注", "时间"})
	for _, row := range rows {
		typ := stringValue(row["type"])
		subType := stringValue(row["sub_type"])
		if label := typeMap[typ]; label != "" {
			typ = label
		}
		if label := subTypeMap[subType]; label != "" {
			subType = label
		}
		_ = writer.Write([]string{
			cellString(row["id"]),
			typ,
			subType,
			cellString(row["amount"]),
			cellString(row["duration"]),
			stringValue(row["color"]),
			stringValue(row["consistency"]),
			cellString(row["temperature"]),
			stringValue(row["note"]),
			stringValue(row["timestamp"]),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, "导出失败")
		return
	}
	filename := "baby_records_" + today().Format("2006-01-02") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(output.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Bytes())
	a.addLog(r.Context(), apiUser(r), "导出CSV", "data", 0,
		fmt.Sprintf("%d条记录", len(rows)))
}

func (a *App) backupExport(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(apiUser(r)) {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	payload := backupPayload{
		Version:    1,
		ExportedAt: time.Now().Format(time.RFC3339Nano),
		Tables:     make(map[string]backupTable, len(backupColumns)),
	}
	tableNames := make([]string, 0, len(backupColumns))
	for table := range backupColumns {
		tableNames = append(tableNames, table)
	}
	sort.Strings(tableNames)
	for _, table := range tableNames {
		columns := backupColumns[table]
		rows, err := a.queryMaps(r.Context(),
			"SELECT "+strings.Join(columns, ",")+" FROM "+table)
		if err != nil {
			rows = []map[string]any{}
		}
		payload.Tables[table] = backupTable{Columns: columns, Rows: rows}
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "导出备份失败")
		return
	}
	filename := "baby_backup_" + today().Format("2006-01-02") + ".json"
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
	a.addLog(r.Context(), apiUser(r), "导出备份", "data", 0, "完整数据库备份")
}

func (a *App) backupRestore(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(apiUser(r)) {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "文件解析失败")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "请选择备份文件")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "读取备份文件失败")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload backupPayload
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "文件解析失败: "+err.Error())
		return
	}
	if payload.Version == nil || payload.Tables == nil {
		writeError(w, http.StatusBadRequest, "无效的备份文件格式")
		return
	}

	// Validate every supplied table before changing the database so malformed
	// files cannot leave a partial restore behind.
	for table, data := range payload.Tables {
		allowedColumns, ok := backupColumns[table]
		if !ok {
			continue
		}
		allowed := make(map[string]bool, len(allowedColumns))
		for _, column := range allowedColumns {
			allowed[column] = true
		}
		for _, column := range data.Columns {
			if !allowed[column] {
				writeError(w, http.StatusBadRequest, "备份文件包含无效的数据列")
				return
			}
		}
		if len(data.Rows) > 0 && len(data.Columns) == 0 {
			writeError(w, http.StatusBadRequest, "备份文件缺少数据列")
			return
		}
	}

	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "恢复备份失败")
		return
	}
	defer tx.Rollback()
	counts := make(map[string]int, len(restoreOrder))
	for _, table := range restoreOrder {
		data, ok := payload.Tables[table]
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(r.Context(), "DELETE FROM "+table); err != nil {
			writeError(w, http.StatusInternalServerError, "恢复备份失败")
			return
		}
		placeholders := make([]string, len(data.Columns))
		for index := range placeholders {
			placeholders[index] = "?"
		}
		query := "INSERT OR IGNORE INTO " + table + " (" + strings.Join(data.Columns, ",") +
			") VALUES (" + strings.Join(placeholders, ",") + ")"
		count := 0
		for _, row := range data.Rows {
			values := make([]any, len(data.Columns))
			for index, column := range data.Columns {
				values[index] = row[column]
			}
			result, err := tx.ExecContext(r.Context(), query, values...)
			if err != nil {
				continue
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr == nil {
				count += int(affected)
			}
		}
		counts[table] = count
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "恢复备份失败")
		return
	}
	detail, _ := json.Marshal(counts)
	a.addLog(r.Context(), apiUser(r), "恢复备份", "data", 0, "恢复: "+string(detail))
	writeJSON(w, http.StatusOK, map[string]any{
		"message": "备份已恢复",
		"counts":  counts,
	})
}

func (a *App) clearData(w http.ResponseWriter, r *http.Request) {
	user := apiUser(r)
	if !isAdmin(user) {
		writeError(w, http.StatusForbidden, "无权限")
		return
	}
	if _, err := a.db.ExecContext(r.Context(), "DELETE FROM records"); err != nil {
		writeError(w, http.StatusInternalServerError, "清除数据失败")
		return
	}
	a.addLog(r.Context(), user, "清除数据", "data", 0, "清除所有记录")
	targetDate := strings.TrimSpace(r.URL.Query().Get("date"))
	if targetDate == "" && r.Body != nil {
		var input map[string]any
		if decodeJSON(w, r, &input) == nil {
			targetDate = strings.TrimSpace(stringValue(input["_date"]))
		}
	}
	summary, err := a.todaySummaryData(r.Context(), targetDate, user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "加载今日概览失败")
		return
	}
	summary["message"] = "所有记录已清除"
	writeJSON(w, http.StatusOK, summary)
}

func (a *App) getServiceConfig(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(apiUser(r)) {
		writeError(w, http.StatusForbidden, "仅管理员可以查看服务配置")
		return
	}
	bindAddress, port := a.serviceConfigValues()
	mcpConfig := a.mcpConfigData(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"bind_address":           bindAddress,
		"port":                   port,
		"enabled":                validServiceConfig(bindAddress, port),
		"mcp_enabled":            mcpConfig["enabled"],
		"mcp_listener_available": mcpConfig["listener_available"],
		"mcp_available":          mcpConfig["available"],
	})
}

func (a *App) updateServiceConfig(w http.ResponseWriter, r *http.Request) {
	user := apiUser(r)
	if !isAdmin(user) {
		writeError(w, http.StatusForbidden, "仅管理员可以修改服务配置")
		return
	}
	var input struct {
		BindAddress string `json:"bind_address"`
		Port        string `json:"port"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式无效")
		return
	}
	bindAddress := strings.TrimSpace(input.BindAddress)
	port := strings.TrimSpace(input.Port)
	if (bindAddress == "") != (port == "") {
		writeError(w, http.StatusBadRequest, "监听地址和端口需同时填写，留空则关闭额外监听")
		return
	}
	if bindAddress != "" {
		address := net.ParseIP(bindAddress)
		if address == nil || address.To4() == nil || address.IsMulticast() {
			writeError(w, http.StatusBadRequest, "监听地址必须是有效的 IPv4 地址")
			return
		}
		bindAddress = address.To4().String()
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1024 || portNumber > 65535 {
			writeError(w, http.StatusBadRequest, "监听端口范围应为 1024-65535")
			return
		}
		port = strconv.Itoa(portNumber)
	}
	launcher := a.cfg.LauncherPath
	if launcher == "" && a.cfg.WebDir != "" {
		candidate := filepath.Join(filepath.Dir(a.cfg.WebDir), "cmd", "main")
		if _, err := os.Stat(candidate); err == nil {
			launcher = candidate
		}
	}
	if launcher == "" {
		writeError(w, http.StatusInternalServerError, "无法定位应用启动器，请从飞牛应用中心停止并重新启动应用")
		return
	}
	currentBind, currentPort := a.serviceConfigValues()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, launcher, "configure")
	command.Env = append(os.Environ(),
		"wizard_bind_address="+bindAddress,
		"wizard_port="+port,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "监听配置校验失败，请确认应用运行正常"
		}
		writeError(w, http.StatusBadRequest, message)
		return
	}
	restarted := currentBind != bindAddress || currentPort != port
	if restarted {
		restart := exec.Command(launcher, "restart")
		restart.Env = append(os.Environ(),
			"wizard_bind_address="+bindAddress,
			"wizard_port="+port,
		)
		if err := restart.Start(); err != nil {
			writeError(w, http.StatusInternalServerError, "保存监听配置后重启失败")
			return
		}
	}
	a.addLog(r.Context(), user, "修改监听配置", "service", 0,
		fmt.Sprintf("%s:%s", bindAddress, port))
	message := "监听配置已保存"
	if !restarted {
		message = "监听配置未变化"
	}
	mcpConfig := a.mcpConfigData(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"message":                message,
		"bind_address":           bindAddress,
		"port":                   port,
		"restarted":              restarted,
		"mcp_enabled":            mcpConfig["enabled"],
		"mcp_listener_available": mcpConfig["listener_available"],
		"mcp_available":          mcpConfig["available"],
	})
}

func (a *App) serviceConfigValues() (string, string) {
	read := func(name, fallback string) string {
		if a.cfg.EtcDir == "" {
			return fallback
		}
		raw, err := os.ReadFile(filepath.Join(a.cfg.EtcDir, name))
		if err != nil {
			return fallback
		}
		value := strings.TrimSpace(string(raw))
		if value == "" {
			return fallback
		}
		return value
	}
	return read("bind_address", strings.TrimSpace(os.Getenv("BABY_TRACKER_BIND_ADDRESS"))),
		read("service_port", strings.TrimSpace(os.Getenv("BABY_TRACKER_SERVICE_PORT")))
}

func validServiceConfig(bindAddress, port string) bool {
	address := net.ParseIP(strings.TrimSpace(bindAddress))
	if address == nil || address.To4() == nil || address.IsMulticast() {
		return false
	}
	portNumber, err := strconv.Atoi(strings.TrimSpace(port))
	return err == nil && portNumber >= 1024 && portNumber <= 65535
}

func cellString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case float64:
		return formatNumber(typed)
	case float32:
		return formatNumber(float64(typed))
	default:
		return fmt.Sprint(typed)
	}
}
