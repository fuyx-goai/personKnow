package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

var layeredDomains = []string{
	"account", "chat", "document", "indexing", "library", "migration", "usage",
}

func TestDomainRootsContainNoGoImplementations(t *testing.T) {
	root := filepath.Join("..")
	for _, domain := range layeredDomains {
		entries, err := os.ReadDir(filepath.Join(root, domain))
		if err != nil {
			t.Fatalf("read domain %s: %v", domain, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" {
				t.Errorf("internal/%s/%s violates AGENTS.md: move it into entity/handler/repo/service", domain, entry.Name())
			}
		}
	}
}

func TestRequiredGatewayLayersExist(t *testing.T) {
	for _, layer := range []string{"handler", "request", "router", "web"} {
		info, err := os.Stat(filepath.Join("..", "gateway", layer))
		if err != nil || !info.IsDir() {
			t.Errorf("internal/gateway/%s must exist", layer)
		}
	}
}

func TestLayerPackageNamesMatchDirectories(t *testing.T) {
	root := filepath.Join("..")
	for _, domain := range layeredDomains {
		for _, layer := range []string{"entity", "handler", "repo", "service"} {
			directory := filepath.Join(root, domain, layer)
			entries, err := os.ReadDir(directory)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatalf("read layer %s/%s: %v", domain, layer, err)
			}
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
					continue
				}
				assertPackageName(t, filepath.Join(directory, entry.Name()), layer)
			}
		}
	}
}

func assertPackageName(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	prefix := []byte("package " + expected + "\n")
	if len(data) < len(prefix) || string(data[:len(prefix)]) != string(prefix) {
		t.Errorf("%s must declare package %s", path, expected)
	}
}
