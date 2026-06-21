package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

// Run on the target host (e.g. EC2) to verify nginx layout discovery:
//
//	go test ./internal/handlers -run TestNginxLayoutEnvironment -v
//
// Or from the repo root:
//
//	go test -v -run TestNginxLayoutEnvironment ./internal/handlers/
func TestNginxLayoutEnvironment(t *testing.T) {
	if _, err := exec.LookPath("nginx"); err != nil {
		t.Skip("nginx not found in PATH; skipping environment check")
	}

	report, err := InspectNginxLayout()
	if err != nil {
		t.Fatalf("InspectNginxLayout failed: %v", err)
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	t.Logf("nginx layout report:\n%s", encoded)

	if report.WriteDir == "" {
		t.Fatal("write_dir is empty")
	}
	if report.Layout == "" {
		t.Fatal("layout is empty")
	}
	if report.Source == "" {
		t.Fatal("source is empty")
	}
	if !report.WriteDirExists {
		t.Errorf("write_dir does not exist: %s", report.WriteDir)
	}
	if !report.WriteDirWritable {
		t.Errorf("write_dir is not writable: %s", report.WriteDir)
	}
	if report.EnableDir != "" && !report.EnableDirExists {
		t.Errorf("enable_dir does not exist: %s", report.EnableDir)
	}
	if !report.NginxTestOK {
		t.Errorf("nginx -t failed:\n%s", report.NginxTestOutput)
	}

	switch report.Layout {
	case "conf.d", "sites-available", "override":
	default:
		t.Errorf("unexpected layout %q", report.Layout)
	}

	if report.NGINXConfDirOverride != "" {
		t.Logf("NGINX_CONF_DIR override is active: %s", report.NGINXConfDirOverride)
	}

	if os.Getenv("NGINX_LAYOUT_ENV_STRICT") == "1" && report.Layout == "override" {
		t.Fatal("NGINX_CONF_DIR override is set; unset it to verify auto-discovery")
	}
}
