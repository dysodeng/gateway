package discovery

import (
	"os"
	"testing"

	"github.com/dysodeng/gateway/pkg/logger"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/api/v3/mvccpb"
)

func TestMain(m *testing.M) {
	logger.InitLogger(false)
	os.Exit(m.Run())
}

func TestEtcdDiscovery_parseKey(t *testing.T) {
	d := &EtcdDiscovery{prefix: "/services/"}

	tests := []struct {
		key         string
		wantService string
		wantID      string
	}{
		{"/services/user-svc/inst-1", "user-svc", "inst-1"},
		{"/services/order-svc/10.0.0.1:8080", "order-svc", "10.0.0.1:8080"},
		{"/services/", "", ""},          // 缺少 serviceName 和 instanceID
		{"/services/user-svc", "", ""},  // 缺少 instanceID
		{"/services/user-svc/", "", ""}, // instanceID 为空
		{"/other/user-svc/inst-1", "", ""},  // 前缀不匹配
	}

	for _, tt := range tests {
		svc, id := d.parseKey(tt.key)
		if svc != tt.wantService || id != tt.wantID {
			t.Errorf("parseKey(%q) = (%q, %q), want (%q, %q)",
				tt.key, svc, id, tt.wantService, tt.wantID)
		}
	}
}

func TestEtcdDiscovery_parseValue(t *testing.T) {
	d := &EtcdDiscovery{}

	value := []byte(`{"host":"10.0.0.1","port":8081,"weight":2,"metadata":{"version":"v2"}}`)
	inst, err := d.parseValue("user-svc", "inst-1", value)
	if err != nil {
		t.Fatalf("parseValue() error: %v", err)
	}
	if inst.ID != "inst-1" {
		t.Errorf("ID = %q, want %q", inst.ID, "inst-1")
	}
	if inst.Host != "10.0.0.1" {
		t.Errorf("Host = %q, want %q", inst.Host, "10.0.0.1")
	}
	if inst.Port != 8081 {
		t.Errorf("Port = %d, want 8081", inst.Port)
	}
	if inst.Weight != 2 {
		t.Errorf("Weight = %d, want 2", inst.Weight)
	}
	if inst.Metadata["version"] != "v2" {
		t.Errorf("Metadata[version] = %q, want %q", inst.Metadata["version"], "v2")
	}
}

func TestEtcdDiscovery_parseValue_InvalidJSON(t *testing.T) {
	d := &EtcdDiscovery{}

	_, err := d.parseValue("svc", "id", []byte("not-json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestEtcdDiscovery_GetInstances_FromCache(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Weight: 1, Status: StatusUp},
				{ID: "inst-2", Name: "user-svc", Host: "10.0.0.2", Port: 8082, Weight: 2, Status: StatusUp},
			},
		},
	}

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
}

func TestEtcdDiscovery_GetInstances_NotFound(t *testing.T) {
	d := &EtcdDiscovery{
		instances: make(map[string][]ServiceInstance),
	}

	_, err := d.GetInstances("unknown-svc")
	if err == nil {
		t.Fatal("expected error for unknown service, got nil")
	}
}

func TestEtcdDiscovery_GetInstances_ReturnsCopy(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"svc": {
				{ID: "1", Name: "svc", Host: "10.0.0.1", Port: 8080, Status: StatusUp, Metadata: map[string]string{"k": "v"}},
			},
		},
	}

	result, _ := d.GetInstances("svc")
	result[0].Host = "mutated"
	result[0].Metadata["k"] = "mutated"

	original, _ := d.GetInstances("svc")
	if original[0].Host == "mutated" {
		t.Fatal("GetInstances 未返回 slice 副本")
	}
	if original[0].Metadata["k"] == "mutated" {
		t.Fatal("GetInstances 未深拷贝 Metadata map")
	}
}

func TestEtcdDiscovery_handleEvent_Put(t *testing.T) {
	d := &EtcdDiscovery{
		prefix:    "/services/",
		instances: make(map[string][]ServiceInstance),
	}

	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/services/user-svc/inst-1"),
			Value: []byte(`{"host":"10.0.0.1","port":8081,"weight":1}`),
		},
	})

	instances, err := d.GetInstances("user-svc")
	if err != nil {
		t.Fatalf("GetInstances() error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].Host != "10.0.0.1" {
		t.Errorf("Host = %q, want %q", instances[0].Host, "10.0.0.1")
	}
}

func TestEtcdDiscovery_handleEvent_PutUpdate(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Weight: 1, Status: StatusUp},
			},
		},
	}

	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/services/user-svc/inst-1"),
			Value: []byte(`{"host":"10.0.0.2","port":9090,"weight":3}`),
		},
	})

	instances, _ := d.GetInstances("user-svc")
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].Host != "10.0.0.2" || instances[0].Port != 9090 {
		t.Errorf("instance not updated: %+v", instances[0])
	}
}

func TestEtcdDiscovery_handleEvent_Delete(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Status: StatusUp},
				{ID: "inst-2", Name: "user-svc", Host: "10.0.0.2", Port: 8082, Status: StatusUp},
			},
		},
	}

	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/services/user-svc/inst-1"),
		},
	})

	instances, _ := d.GetInstances("user-svc")
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	if instances[0].ID != "inst-2" {
		t.Errorf("wrong instance remaining: %+v", instances[0])
	}
}

func TestEtcdDiscovery_handleEvent_DeleteLast(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Status: StatusUp},
			},
		},
	}

	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypeDelete,
		Kv: &mvccpb.KeyValue{
			Key: []byte("/services/user-svc/inst-1"),
		},
	})

	_, err := d.GetInstances("user-svc")
	if err == nil {
		t.Fatal("expected error after deleting last instance, got nil")
	}
}

func TestEtcdDiscovery_GetInstances_FiltersNonUpStatus(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Status: StatusUp},
				{ID: "inst-2", Name: "user-svc", Host: "10.0.0.2", Port: 8082, Status: StatusDown},
				{ID: "inst-3", Name: "user-svc", Host: "10.0.0.3", Port: 8083, Status: StatusDraining},
			},
		},
	}

	instances, err := d.GetInstances("user-svc")
	if err != nil {
		t.Fatalf("GetInstances() error: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1 (only up)", len(instances))
	}
	if instances[0].ID != "inst-1" {
		t.Errorf("expected inst-1, got %s", instances[0].ID)
	}
}

func TestEtcdDiscovery_GetInstances_AllDown(t *testing.T) {
	d := &EtcdDiscovery{
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Status: StatusDown},
			},
		},
	}

	_, err := d.GetInstances("user-svc")
	if err == nil {
		t.Fatal("expected error when all instances are down, got nil")
	}
}

func TestEtcdDiscovery_parseValue_EmptyStatusDefaultsToUp(t *testing.T) {
	d := &EtcdDiscovery{}

	value := []byte(`{"host":"10.0.0.1","port":8081,"weight":1,"status":""}`)
	inst, err := d.parseValue("svc", "id", value)
	if err != nil {
		t.Fatalf("parseValue() error: %v", err)
	}
	if inst.Status != StatusUp {
		t.Errorf("Status = %q, want %q", inst.Status, StatusUp)
	}
}

func TestEtcdDiscovery_handleEvent_StatusChange(t *testing.T) {
	d := &EtcdDiscovery{
		prefix: "/services/",
		instances: map[string][]ServiceInstance{
			"user-svc": {
				{ID: "inst-1", Name: "user-svc", Host: "10.0.0.1", Port: 8081, Status: StatusUp},
			},
		},
	}

	// 将实例状态改为 down
	d.handleEvent(&clientv3.Event{
		Type: clientv3.EventTypePut,
		Kv: &mvccpb.KeyValue{
			Key:   []byte("/services/user-svc/inst-1"),
			Value: []byte(`{"host":"10.0.0.1","port":8081,"weight":1,"status":"down"}`),
		},
	})

	// GetInstances 应过滤掉 down 状态的实例
	_, err := d.GetInstances("user-svc")
	if err == nil {
		t.Fatal("expected error when instance is down, got nil")
	}
}
