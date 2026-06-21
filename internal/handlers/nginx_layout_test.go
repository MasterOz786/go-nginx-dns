package handlers

import "testing"

func TestLayoutFromIncludesConfD(t *testing.T) {
	layout, ok := layoutFromIncludes([]string{
		"/etc/nginx/mime.types",
		"/etc/nginx/conf.d/*.conf",
	}, "/etc/nginx")
	if !ok {
		t.Fatal("expected layout match")
	}
	if layout.Layout != "conf.d" {
		t.Fatalf("layout = %q, want conf.d", layout.Layout)
	}
	if layout.WriteDir != "/etc/nginx/conf.d" {
		t.Fatalf("WriteDir = %q", layout.WriteDir)
	}
	if layout.EnableDir != "" {
		t.Fatalf("EnableDir = %q, want empty", layout.EnableDir)
	}
}

func TestLayoutFromIncludesSitesEnabled(t *testing.T) {
	layout, ok := layoutFromIncludes([]string{
		"/etc/nginx/sites-enabled/*",
	}, "/etc/nginx")
	if !ok {
		t.Fatal("expected layout match")
	}
	if layout.Layout != "sites-available" {
		t.Fatalf("layout = %q, want sites-available", layout.Layout)
	}
	if layout.WriteDir != "/etc/nginx/sites-available" {
		t.Fatalf("WriteDir = %q", layout.WriteDir)
	}
	if layout.EnableDir != "/etc/nginx/sites-enabled" {
		t.Fatalf("EnableDir = %q", layout.EnableDir)
	}
}

func TestLayoutFromIncludesPrefersSitesEnabled(t *testing.T) {
	layout, ok := layoutFromIncludes([]string{
		"/etc/nginx/conf.d/*.conf",
		"/etc/nginx/sites-enabled/*",
	}, "/etc/nginx")
	if !ok {
		t.Fatal("expected layout match")
	}
	if layout.Layout != "sites-available" {
		t.Fatalf("layout = %q, want sites-available when both are present", layout.Layout)
	}
}

func TestIncludeDir(t *testing.T) {
	got := includeDir("/etc/nginx/conf.d/*.conf", "/etc/nginx")
	if got != "/etc/nginx/conf.d" {
		t.Fatalf("includeDir = %q", got)
	}
}
