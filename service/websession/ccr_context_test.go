package websession

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestLoadCCRModelCatalogParsesGatewayModels(t *testing.T) {
	var gotAuth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"ZCode API/GLM-5.3","object":"model","owned_by":"ZCode API"},
			{"id":"DeepSeek/deepseek-flash","object":"model","owned_by":"DeepSeek","display_name":"DeepSeek V4.1 Flash"}
		]}`))
	}))
	defer server.Close()

	keyPath := filepath.Join(t.TempDir(), "ccr-claude-code-api-key-default-claude-code.cmd")
	if err := os.WriteFile(keyPath, []byte("@echo off\r\necho ccr-profile-token-123\r\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	models, err := loadCCRModelCatalog(context.Background(), server.URL+"/", keyPath)
	if err != nil {
		t.Fatalf("loadCCRModelCatalog returned error: %v", err)
	}
	if gotAuth.Load() != "Bearer ccr-profile-token-123" {
		t.Fatalf("expected profile api key auth header, got %v", gotAuth.Load())
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].Model != "ZCode API/GLM-5.3" || models[0].Provider != "ZCode API" || models[0].DisplayName != "" {
		t.Fatalf("unexpected first model: %+v", models[0])
	}
	if models[1].Model != "DeepSeek/deepseek-flash" || models[1].DisplayName != "DeepSeek V4.1 Flash" {
		t.Fatalf("unexpected second model: %+v", models[1])
	}
}

func TestLoadCCRModelCatalogRequiresGateway(t *testing.T) {
	if _, err := loadCCRModelCatalog(context.Background(), "", ""); err == nil {
		t.Fatal("expected error when gateway url is not configured")
	}
}

func TestReadCCRProfileAPIKeyStripsEchoWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ccr-claude-code-api-key-default-claude-code.cmd")
	if err := os.WriteFile(path, []byte("@echo off\r\necho \"ccr-profile-key-42\"\r\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if key := readCCRProfileAPIKey(path); key != "ccr-profile-key-42" {
		t.Fatalf("expected stripped key, got %q", key)
	}
	if key := readCCRProfileAPIKey(filepath.Join(dir, "missing.cmd")); key != "" {
		t.Fatalf("expected empty key for missing file, got %q", key)
	}
}

func TestCCRProfileAPIKeyPathResolvesSlugCandidates(t *testing.T) {
	dir := t.TempDir()
	ccrPath := filepath.Join(dir, "ccr-app.cmd")
	if err := os.WriteFile(ccrPath, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write fake ccr launcher: %v", err)
	}
	idPath := filepath.Join(dir, "ccr-claude-code-api-key-default-claude-code.cmd")
	if err := os.WriteFile(idPath, []byte("@echo off\r\necho ccr-profile-token\r\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	if got := ccrProfileAPIKeyPath(ccrPath, "default-claude-code"); got != idPath {
		t.Fatalf("expected exact id key path %q, got %q", idPath, got)
	}
	if got := ccrProfileAPIKeyPath(ccrPath, "missing-profile"); got != "" {
		t.Fatalf("expected empty path for unknown profile, got %q", got)
	}

	namePath := filepath.Join(dir, "ccr-claude-code-api-key-claude-code.cmd")
	if err := os.WriteFile(namePath, []byte("@echo off\r\necho ccr-profile-token\r\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	// Windows filesystems match paths case-insensitively, so compare loosely.
	if got := ccrProfileAPIKeyPath(ccrPath, "Claude Code"); !strings.EqualFold(got, namePath) {
		t.Fatalf("expected normalized name key path %q, got %q", namePath, got)
	}
}

func TestGetCCRModelCatalogCachesGatewayResponses(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"id":"ZCode API/GLM-5.3","owned_by":"ZCode API"},
			{"id":"天才程序员CC/glm-5.3-flash","owned_by":"天才程序员CC"}
		]}`))
	}))
	defer server.Close()

	binDir := t.TempDir()
	ccrPath := filepath.Join(binDir, "ccr-app.cmd")
	if err := os.WriteFile(ccrPath, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write fake ccr launcher: %v", err)
	}
	keyPath := filepath.Join(binDir, "ccr-claude-code-api-key-default-claude-code.cmd")
	if err := os.WriteFile(keyPath, []byte("@echo off\r\necho ccr-profile-token\r\n"), 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	manager := &Manager{cfg: Config{CCRGatewayURL: server.URL, CCRPath: ccrPath, CCRProfile: "default-claude-code"}}

	models := manager.getCCRModelCatalog(false)
	if len(models) != 2 {
		t.Fatalf("expected the full gateway catalog without filtering, got %+v", models)
	}
	if models[0].Model != "ZCode API/GLM-5.3" || models[1].Model != "天才程序员CC/glm-5.3-flash" {
		t.Fatalf("unexpected catalog: %+v", models)
	}
	if again := manager.getCCRModelCatalog(false); len(again) != 2 || again[0].Model != models[0].Model {
		t.Fatalf("unexpected cached catalog: %+v", again)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected cached probe to hit gateway once, got %d requests", requests.Load())
	}

	if forced := manager.getCCRModelCatalog(true); len(forced) != 2 {
		t.Fatalf("unexpected forced catalog: %+v", forced)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected forced probe to hit gateway again, got %d requests", requests.Load())
	}
}
