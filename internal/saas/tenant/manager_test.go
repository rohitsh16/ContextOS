package tenant

import (
	"strings"
	"testing"
	"time"
)

func TestSharedProcessTenantIsolation(t *testing.T) {
	baseDir := t.TempDir()

	mgr, err := NewManager(Options{
		BaseDataDir: baseDir,
		StorageType: "file",
		IdleTTL:     10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Close()

	// Get services for two distinct tenants
	svcA, err := mgr.GetService("tenant_alpha")
	if err != nil {
		t.Fatalf("GetService tenant_alpha failed: %v", err)
	}

	svcB, err := mgr.GetService("tenant_beta")
	if err != nil {
		t.Fatalf("GetService tenant_beta failed: %v", err)
	}

	if svcA == svcB {
		t.Fatal("tenant services must be independent instances")
	}

	// Seed tenant-specific memories
	_, err = svcA.Remember("secret", "Alpha AWS credentials access key AKIAALPHA", "user", "repo", "", 0.99, nil)
	if err != nil {
		t.Fatalf("svcA Remember failed: %v", err)
	}

	_, err = svcB.Remember("secret", "Beta GCP project token TOKENBETA", "user", "repo", "", 0.99, nil)
	if err != nil {
		t.Fatalf("svcB Remember failed: %v", err)
	}

	// Plan on Tenant Alpha
	planA, err := svcA.Plan("AWS credentials access key", "", 4000)
	if err != nil {
		t.Fatalf("svcA Plan failed: %v", err)
	}

	for _, item := range planA.Selected {
		if strings.Contains(item.Content, "Beta GCP") || strings.Contains(item.Content, "TOKENBETA") {
			t.Fatalf("ISOLATION LEAK: Tenant Alpha saw Tenant Beta secret: %s", item.Content)
		}
	}

	// Plan on Tenant Beta
	planB, err := svcB.Plan("GCP project token", "", 4000)
	if err != nil {
		t.Fatalf("svcB Plan failed: %v", err)
	}

	for _, item := range planB.Selected {
		if strings.Contains(item.Content, "Alpha AWS") || strings.Contains(item.Content, "AKIAALPHA") {
			t.Fatalf("ISOLATION LEAK: Tenant Beta saw Tenant Alpha secret: %s", item.Content)
		}
	}

	// Verify active tenant count
	if mgr.ActiveTenantCount() != 2 {
		t.Fatalf("expected 2 active tenants, got %d", mgr.ActiveTenantCount())
	}

	// Evict one tenant
	if err := mgr.Evict("tenant_alpha"); err != nil {
		t.Fatalf("Evict failed: %v", err)
	}
	if mgr.ActiveTenantCount() != 1 {
		t.Fatalf("expected 1 active tenant after eviction, got %d", mgr.ActiveTenantCount())
	}
}
