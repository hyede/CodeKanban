package websession

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	ccrModelCatalogCacheTTL = 5 * time.Minute
	ccrModelCatalogTimeout  = 3 * time.Second
)

type ccrModelCatalogCache = runtimeCapabilityCache[[]CCRModelInfo]

// CCRModelInfo describes one model exposed by the Claude Code Router gateway.
// Model carries the full gateway id ("Provider/model") that must be passed to
// claude via --model so the gateway routes to the matching provider.
type CCRModelInfo struct {
	Model       string `json:"model"`
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName,omitempty"`
}

func (m *Manager) getCCRModelCatalog(force bool) []CCRModelInfo {
	if m == nil {
		return nil
	}
	models := m.ccrModels.get(
		force,
		runtimeCapabilityCachePolicy{successTTL: ccrModelCatalogCacheTTL},
		cloneCCRModelCatalog,
		m.probeCCRModelCatalog,
	)
	return models
}

func (m *Manager) getCCRModelCatalogBackground() ([]CCRModelInfo, bool) {
	if m == nil {
		return nil, false
	}
	return m.ccrModels.getBackground(
		runtimeCapabilityCachePolicy{successTTL: ccrModelCatalogCacheTTL},
		cloneCCRModelCatalog,
		m.probeCCRModelCatalog,
	)
}

func (m *Manager) probeCCRModelCatalog() ([]CCRModelInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ccrModelCatalogTimeout)
	defer cancel()
	apiKeyPath := ccrProfileAPIKeyPath(m.cfg.CCRPath, m.cfg.CCRProfile)
	models, err := loadCCRModelCatalog(ctx, m.cfg.CCRGatewayURL, apiKeyPath)
	if err != nil {
		return nil, err
	}
	if models == nil {
		models = []CCRModelInfo{}
	}
	return models, nil
}

// ccrRuntimeAvailable reports whether a CCR launcher can be resolved, so the
// gateway model catalog probe is worth running.
func (m *Manager) ccrRuntimeAvailable() bool {
	if m == nil {
		return false
	}
	ccrPath := strings.TrimSpace(m.cfg.CCRPath)
	if ccrPath == "" {
		ccrPath = defaultCCRPath()
	}
	if _, err := exec.LookPath(ccrPath); err == nil {
		return true
	}
	if info, err := os.Stat(ccrPath); err == nil && !info.IsDir() {
		return true
	}
	return false
}

func cloneCCRModelCatalog(models []CCRModelInfo) []CCRModelInfo {
	if models == nil {
		return nil
	}
	out := make([]CCRModelInfo, len(models))
	copy(out, models)
	return out
}

type ccrModelsResponse struct {
	Data []struct {
		ID          string `json:"id"`
		OwnedBy     string `json:"owned_by"`
		DisplayName string `json:"display_name"`
	} `json:"data"`
}

func loadCCRModelCatalog(ctx context.Context, baseURL, apiKeyPath string) ([]CCRModelInfo, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("ccr gateway url is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	if key := readCCRProfileAPIKey(apiKeyPath); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ccr gateway returned status %d", resp.StatusCode)
	}
	var payload ccrModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	models := make([]CCRModelInfo, 0, len(payload.Data))
	for _, entry := range payload.Data {
		model := strings.TrimSpace(entry.ID)
		if model == "" {
			continue
		}
		provider := strings.TrimSpace(entry.OwnedBy)
		if provider == "" {
			if index := strings.LastIndex(model, "/"); index > 0 {
				provider = model[:index]
			}
		}
		models = append(models, CCRModelInfo{
			Model:       model,
			Provider:    provider,
			DisplayName: strings.TrimSpace(entry.DisplayName),
		})
	}
	return models, nil
}

// readCCRProfileAPIKey extracts the gateway key echoed by the launcher script
// the desktop app writes next to its bin directory (e.g. a line
// `echo ccr-profile-...` inside a .cmd/.sh wrapper).
func readCCRProfileAPIKey(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		line = strings.TrimPrefix(line, "echo ")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, `"'`)
		if line != "" && !strings.HasPrefix(line, "@") && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// ccrProfileAPIKeyPath locates the api-key launcher for the configured profile
// inside the CCR bin directory. The desktop app regenerates these per profile.
func ccrProfileAPIKeyPath(ccrPath, profile string) string {
	dir := ccrLauncherBinDir(ccrPath)
	if dir == "" {
		return ""
	}
	base := "ccr-claude-code-api-key-"
	for _, name := range ccrProfileFileCandidates(profile) {
		for _, ext := range []string{"", ".cmd"} {
			candidate := filepath.Join(dir, base+name+ext)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

// ccrProfileFileCandidates mirrors the desktop app's slug normalization: the
// profile id is used verbatim when it is already filesystem-safe, and a
// normalized fallback covers display names such as "Claude Code".
func ccrProfileFileCandidates(profile string) []string {
	trimmed := strings.TrimSpace(profile)
	if trimmed == "" {
		return nil
	}
	candidates := []string{trimmed}
	var normalized strings.Builder
	for _, char := range trimmed {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9',
			char == '_', char == '.', char == '-':
			normalized.WriteRune(char)
		default:
			normalized.WriteRune('-')
		}
	}
	slug := strings.Trim(normalized.String(), "-")
	if slug != trimmed {
		candidates = append(candidates, slug)
	}
	lower := strings.ToLower(slug)
	if lower != slug {
		candidates = append(candidates, lower)
	}
	return candidates
}

// ccrLauncherBinDir resolves the bin directory that holds the CCR launchers:
// the directory of the resolved ccr command, or the desktop app's install
// directory when the command is not on the (possibly stale) PATH.
func ccrLauncherBinDir(ccrPath string) string {
	if trimmed := strings.TrimSpace(ccrPath); trimmed != "" {
		if resolved, err := exec.LookPath(trimmed); err == nil && resolved != "" {
			return filepath.Dir(resolved)
		}
		if filepath.IsAbs(trimmed) {
			return filepath.Dir(trimmed)
		}
	}
	for _, dir := range ccrDesktopBinDirs() {
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			return dir
		}
	}
	return ""
}
