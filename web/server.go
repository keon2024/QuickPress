package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"quickpress/concurrency"
	"quickpress/config"
)

//go:embed static/index.html
var staticFiles embed.FS

type Server struct {
	manager           *concurrency.Manager
	defaultConfigPath string
}

func New(manager *concurrency.Manager, defaultConfigPath string) http.Handler {
	s := &Server{manager: manager, defaultConfigPath: defaultConfigPath}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/config/default", s.handleDefaultConfig)
	mux.HandleFunc("/api/config/import", s.handleConfigImport)
	mux.HandleFunc("/api/config/export", s.handleConfigExport)
	mux.HandleFunc("/api/reader/upload", s.handleReaderUpload)
	mux.HandleFunc("/api/run/start", s.handleRunStart)
	mux.HandleFunc("/api/run/stop", s.handleRunStop)
	mux.HandleFunc("/api/run/status", s.handleRunStatus)
	mux.HandleFunc("/api/run/results", s.handleRunResults)
	mux.HandleFunc("/api/run/adjust", s.handleRunAdjust)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleDefaultConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	cfg := config.Default()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":   s.defaultConfigPath,
		"config": cfg,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleConfigGet(w, r)
	case http.MethodPost:
		s.handleConfigSave(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	path := s.resolveConfigPath(r.URL.Query().Get("path"))
	cfg, err := config.Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg = config.Default()
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":   path,
		"config": cfg,
	})
}

func (s *Server) handleConfigSave(w http.ResponseWriter, r *http.Request) {
	path := s.resolveConfigPath(r.URL.Query().Get("path"))
	if strings.TrimSpace(path) == "" {
		writeError(w, http.StatusBadRequest, "请提供配置文件路径")
		return
	}

	var payload struct {
		Config config.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload.Config.Normalize()
	if err := config.Save(path, payload.Config); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"saved":  true,
		"path":   path,
		"config": payload.Config,
	})
}

func (s *Server) handleConfigImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Content  string `json:"content"`
		FileName string `json:"file_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(payload.Content) == "" {
		writeError(w, http.StatusBadRequest, "导入文件内容不能为空")
		return
	}
	cfg, err := config.Parse([]byte(payload.Content))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"imported":  true,
		"file_name": strings.TrimSpace(payload.FileName),
		"config":    cfg,
	})
}

func (s *Server) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Path     string        `json:"path"`
		FileName string        `json:"file_name"`
		Config   config.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	content, err := config.Marshal(payload.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"file_name": inferExportFileName(payload.Path, payload.FileName, s.defaultConfigPath),
		"content":   string(content),
	})
}

func (s *Server) handleReaderUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Content  string `json:"content"`
		FileName string `json:"file_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fileName := safeUploadFileName(payload.FileName, "dataset.csv")
	uploadDir := filepath.Join(filepath.Dir(s.defaultConfigPath), "data")
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	storedPath := filepath.Join(uploadDir, fileName)
	if err := os.WriteFile(storedPath, []byte(payload.Content), 0o644); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uploaded":  true,
		"file_name": fileName,
		"path":      toWorkspaceRelativePath(storedPath),
	})
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Path   string        `json:"path"`
		Config config.Config `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload.Path = s.resolveConfigPath(payload.Path)
	cfg := payload.Config
	if len(cfg.Requests) == 0 && strings.TrimSpace(payload.Path) != "" {
		loaded, err := config.Load(payload.Path)
		if err == nil {
			cfg = loaded
		}
	}
	cfg.Normalize()
	if err := s.manager.Start(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"started": true,
		"status":  s.manager.Status(),
	})
}

func (s *Server) handleRunStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.manager.Stop(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"stopped": true,
		"status":  s.manager.Status(),
	})
}

func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *Server) handleRunResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := 120
	failuresOnly := parseBoolQuery(r.URL.Query().Get("failures"))
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "limit 必须是整数")
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": s.manager.Results(limit, failuresOnly),
	})
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func (s *Server) handleRunAdjust(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload struct {
		Target *int           `json:"target"`
		Stages []config.Stage `json:"stages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if payload.Target == nil && len(payload.Stages) == 0 {
		writeError(w, http.StatusBadRequest, "请至少提供 target 或 stages")
		return
	}
	if payload.Target != nil {
		if err := s.manager.AdjustTarget(*payload.Target); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(payload.Stages) > 0 {
		if err := s.manager.ReplaceStages(payload.Stages); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"updated": true,
		"status":  s.manager.Status(),
	})
}

func (s *Server) resolveConfigPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return s.defaultConfigPath
	}
	return config.ResolvePath(path)
}

func inferExportFileName(path, fileName, fallback string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		candidate := strings.TrimSpace(path)
		if candidate == "" {
			candidate = strings.TrimSpace(fallback)
		}
		name = filepath.Base(candidate)
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "quickpress-config.yml"
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext == "" || ext == ".json" {
		name = strings.TrimSuffix(name, ext) + ".yml"
	}
	return name
}

func safeUploadFileName(name, fallback string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = fallback
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r >= 0x80:
			return r
		case r == '.', r == '-', r == '_':
			return r
		default:
			return '_'
		}
	}, name)
	if strings.Trim(strings.TrimSpace(name), "._-") == "" {
		name = fallback
	}
	if filepath.Ext(name) == "" {
		name += ".csv"
	}
	return name
}

func toWorkspaceRelativePath(path string) string {
	absolute := strings.TrimSpace(path)
	if absolute == "" {
		return absolute
	}
	wd, err := os.Getwd()
	if err != nil {
		return absolute
	}
	rel, err := filepath.Rel(wd, absolute)
	if err != nil || strings.HasPrefix(rel, "..") {
		return absolute
	}
	return filepath.ToSlash(rel)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": message,
	})
}
