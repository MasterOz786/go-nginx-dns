package handlers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// nginxLayout describes where vhost configs should be written on this host.
type nginxLayout struct {
	WriteDir  string // primary config file directory
	EnableDir string // non-empty when a symlink into sites-enabled is required
	Layout    string // "conf.d", "sites-available", or "override"
	Source    string // how the layout was determined (for responses/debugging)
}

var includePattern = regexp.MustCompile(`(?i)include\s+([^;]+);`)

// NginxLayoutReport describes how nginx vhost configs are discovered on this host.
type NginxLayoutReport struct {
	MainConf             string   `json:"main_conf"`
	NginxRoot            string   `json:"nginx_root"`
	Includes             []string `json:"includes"`
	WriteDir             string   `json:"write_dir"`
	EnableDir            string   `json:"enable_dir,omitempty"`
	Layout               string   `json:"layout"`
	Source               string   `json:"source"`
	WriteDirExists       bool     `json:"write_dir_exists"`
	WriteDirWritable     bool     `json:"write_dir_writable"`
	EnableDirExists      bool     `json:"enable_dir_exists"`
	SampleConfigPath     string   `json:"sample_config_path"`
	NGINXConfDirOverride string   `json:"nginx_conf_dir_override,omitempty"`
	NginxTestOK          bool     `json:"nginx_test_ok"`
	NginxTestOutput      string   `json:"nginx_test_output,omitempty"`
}

// DiscoverNginxLayout finds the correct nginx vhost directory for the running
// installation by inspecting nginx -V output and include directives in nginx.conf.
// NGINX_CONF_DIR, when set, skips discovery and writes directly to that path.
func DiscoverNginxLayout() (nginxLayout, error) {
	return discoverNginxLayout()
}

// InspectNginxLayout probes the local nginx installation and returns a report
// suitable for environment verification.
func InspectNginxLayout() (NginxLayoutReport, error) {
	report := NginxLayoutReport{
		NGINXConfDirOverride: strings.TrimSpace(os.Getenv("NGINX_CONF_DIR")),
	}

	layout, err := discoverNginxLayout()
	if err != nil {
		return report, err
	}

	report.WriteDir = layout.WriteDir
	report.EnableDir = layout.EnableDir
	report.Layout = layout.Layout
	report.Source = layout.Source
	report.SampleConfigPath = filepath.Join(layout.WriteDir, "example.com.conf")
	report.WriteDirExists = dirExists(layout.WriteDir)
	report.WriteDirWritable = dirWritable(layout.WriteDir)
	if layout.EnableDir != "" {
		report.EnableDirExists = dirExists(layout.EnableDir)
	}

	if report.NGINXConfDirOverride == "" {
		if mainConf, confErr := nginxMainConfPath(); confErr == nil {
			report.MainConf = mainConf
			report.NginxRoot = filepath.Dir(mainConf)
			if includes, incErr := collectNginxIncludes(mainConf, report.NginxRoot); incErr == nil {
				report.Includes = includes
			}
		}
	}

	testOut, testErr := exec.Command("nginx", "-t").CombinedOutput()
	report.NginxTestOutput = strings.TrimSpace(string(testOut))
	report.NginxTestOK = testErr == nil

	return report, nil
}

// discoverNginxLayout finds the correct nginx vhost directory for the running
// installation by inspecting nginx -V output and include directives in nginx.conf.
// NGINX_CONF_DIR, when set, skips discovery and writes directly to that path.
func discoverNginxLayout() (nginxLayout, error) {
	if override := strings.TrimSpace(os.Getenv("NGINX_CONF_DIR")); override != "" {
		return nginxLayout{
			WriteDir: override,
			Layout:   "override",
			Source:   "NGINX_CONF_DIR",
		}, nil
	}

	mainConf, err := nginxMainConfPath()
	if err != nil {
		return fallbackNginxLayout(err)
	}

	nginxRoot := filepath.Dir(mainConf)
	includes, err := collectNginxIncludes(mainConf, nginxRoot)
	if err != nil {
		return fallbackNginxLayout(fmt.Errorf("read nginx.conf: %w", err))
	}

	if layout, ok := layoutFromIncludes(includes, nginxRoot); ok {
		return layout, nil
	}

	return fallbackNginxLayout(fmt.Errorf("no vhost include directives found in %s", mainConf))
}

func nginxMainConfPath() (string, error) {
	cmd := exec.Command("nginx", "-V")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return "", fmt.Errorf("nginx -V failed: %w", err)
	}

	for _, field := range strings.Fields(string(out)) {
		if strings.HasPrefix(field, "--conf-path=") {
			path := strings.TrimPrefix(field, "--conf-path=")
			if path != "" {
				return path, nil
			}
		}
	}

	return "/etc/nginx/nginx.conf", nil
}

func collectNginxIncludes(mainConf, nginxRoot string) ([]string, error) {
	seen := make(map[string]struct{})
	var includes []string

	var walk func(path string) error
	walk = func(path string) error {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if _, ok := seen[abs]; ok {
			return nil
		}
		seen[abs] = struct{}{}

		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}

		for _, match := range includePattern.FindAllStringSubmatch(string(data), -1) {
			if len(match) < 2 {
				continue
			}
			pattern := strings.Trim(strings.TrimSpace(match[1]), `"'`)
			includes = append(includes, pattern)

			if strings.Contains(pattern, "*") {
				continue
			}

			resolved := resolveIncludePath(pattern, filepath.Dir(abs), nginxRoot)
			if info, statErr := os.Stat(resolved); statErr == nil && !info.IsDir() {
				if walkErr := walk(resolved); walkErr != nil {
					return walkErr
				}
			}
		}
		return nil
	}

	if err := walk(mainConf); err != nil {
		return nil, err
	}
	return includes, nil
}

func resolveIncludePath(pattern, confDir, nginxRoot string) string {
	if filepath.IsAbs(pattern) {
		return pattern
	}
	if strings.HasPrefix(pattern, "conf/") || strings.HasPrefix(pattern, "conf"+string(os.PathSeparator)) {
		return filepath.Join(nginxRoot, pattern)
	}
	return filepath.Join(confDir, pattern)
}

func layoutFromIncludes(includes []string, nginxRoot string) (nginxLayout, bool) {
	var confDDir string

	for _, pattern := range includes {
		normalized := filepath.ToSlash(strings.ToLower(pattern))
		dir := includeDir(pattern, nginxRoot)

		switch {
		case strings.Contains(normalized, "sites-enabled"):
			writeDir := filepath.Join(nginxRoot, "sites-available")
			enableDir := dir
			if !filepath.IsAbs(enableDir) {
				enableDir = filepath.Join(nginxRoot, "sites-enabled")
			}
			return nginxLayout{
				WriteDir:  writeDir,
				EnableDir: enableDir,
				Layout:    "sites-available",
				Source:    fmt.Sprintf("include %s", pattern),
			}, true

		case strings.Contains(normalized, "sites-available"):
			return nginxLayout{
				WriteDir: dir,
				Layout:   "sites-available",
				Source:   fmt.Sprintf("include %s", pattern),
			}, true

		case strings.Contains(normalized, "conf.d"):
			confDDir = dir
		}
	}

	if confDDir != "" {
		return nginxLayout{
			WriteDir: confDDir,
			Layout:   "conf.d",
			Source:   "include conf.d/*.conf",
		}, true
	}

	return nginxLayout{}, false
}

func includeDir(pattern, nginxRoot string) string {
	cleaned := strings.TrimSpace(pattern)
	if idx := strings.IndexAny(cleaned, "*?["); idx >= 0 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimRight(cleaned, `/\`)

	if cleaned == "" {
		return nginxRoot
	}
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	return filepath.Join(nginxRoot, cleaned)
}

func fallbackNginxLayout(reason error) (nginxLayout, error) {
	nginxRoot := "/etc/nginx"
	candidates := []nginxLayout{
		{
			WriteDir:  filepath.Join(nginxRoot, "sites-available"),
			EnableDir: filepath.Join(nginxRoot, "sites-enabled"),
			Layout:    "sites-available",
			Source:    "fallback: sites-available/sites-enabled",
		},
		{
			WriteDir: filepath.Join(nginxRoot, "conf.d"),
			Layout:   "conf.d",
			Source:   "fallback: conf.d",
		},
	}

	for _, candidate := range candidates {
		if dirExists(candidate.WriteDir) {
			if candidate.EnableDir != "" && !dirExists(candidate.EnableDir) {
				candidate.EnableDir = ""
			}
			if reason != nil {
				candidate.Source = fmt.Sprintf("%s (%v)", candidate.Source, reason)
			}
			return candidate, nil
		}
		if candidate.EnableDir != "" && dirExists(candidate.EnableDir) {
			if reason != nil {
				candidate.Source = fmt.Sprintf("%s (%v)", candidate.Source, reason)
			}
			return candidate, nil
		}
	}

	if reason != nil {
		return nginxLayout{}, fmt.Errorf("could not determine nginx config directory: %w", reason)
	}
	return nginxLayout{}, fmt.Errorf("could not determine nginx config directory")
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func dirWritable(path string) bool {
	if !dirExists(path) {
		return false
	}
	testFile := filepath.Join(path, ".write-test")
	if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
		return false
	}
	_ = os.Remove(testFile)
	return true
}

func writeNginxSiteConfig(layout nginxLayout, configFilename, nginxConfig string) (configPath, enabledPath string, err error) {
	if err = os.MkdirAll(layout.WriteDir, 0755); err != nil {
		return "", "", fmt.Errorf("create %s: %w", layout.WriteDir, err)
	}

	configPath = filepath.Join(layout.WriteDir, configFilename)
	if err = os.WriteFile(configPath, []byte(nginxConfig), 0644); err != nil {
		return "", "", fmt.Errorf("write nginx config: %w", err)
	}

	if layout.EnableDir == "" {
		return configPath, "", nil
	}

	if err = os.MkdirAll(layout.EnableDir, 0755); err != nil {
		return "", "", fmt.Errorf("create %s: %w", layout.EnableDir, err)
	}

	enabledPath = filepath.Join(layout.EnableDir, configFilename)
	if _, statErr := os.Lstat(enabledPath); statErr == nil {
		_ = os.Remove(enabledPath)
	}
	if err = os.Symlink(configPath, enabledPath); err != nil {
		return "", "", fmt.Errorf("enable site in %s: %w", layout.EnableDir, err)
	}

	return configPath, enabledPath, nil
}
