package discovery

import (
	"testing"

	"github.com/dysodeng/gateway/config"
)

func TestStaticDiscovery_GetInstances(t *testing.T) {
	cfg := &config.StaticDiscoveryConfig{
		Services: map[string][]config.StaticInstanceConfig{
			"user-svc": {
				{Host: "127.0.0.1", Port: 8081, Weight: 1},
				{Host: "127.0.0.1", Port: 8082, Weight: 2},
			},
		},
	}

	d := NewStaticDiscovery(cfg)

	instances, err := d.GetInstances("user-svc")
	if err != nil {
		t.Fatalf("GetInstances() error: %v", err)
	}
	if len(instances) != 2 {
		t.Fatalf("got %d instances, want 2", len(instances))
	}
	if instances[0].Port != 8081 {
		t.Errorf("instances[0].Port = %d, want 8081", instances[0].Port)
	}
	if instances[1].Weight != 2 {
		t.Errorf("instances[1].Weight = %d, want 2", instances[1].Weight)
	}
}

func TestStaticDiscovery_GetInstances_NotFound(t *testing.T) {
	cfg := &config.StaticDiscoveryConfig{
		Services: map[string][]config.StaticInstanceConfig{},
	}

	d := NewStaticDiscovery(cfg)

	_, err := d.GetInstances("unknown-svc")
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}
