package balancer

import (
	"path/filepath"
	"testing"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
)

func initCircuitTestDB(t *testing.T) {
	t.Helper()
	if err := db.InitDB("sqlite", filepath.Join(t.TempDir(), "circuit.db"), false); err != nil {
		t.Fatalf("初始化测试数据库失败：%v", err)
	}
	if err := op.InitCache(); err != nil {
		t.Fatalf("初始化测试缓存失败：%v", err)
	}
	globalBreaker.Clear()
	t.Cleanup(func() {
		globalBreaker.Clear()
		_ = db.Close()
	})
}

func TestCircuitBreakerDisabledBypassesAndClearsState(t *testing.T) {
	initCircuitTestDB(t)
	const channelID, keyID = 1, 1
	const modelName = "test-model"

	if err := op.SettingSetString(model.SettingKeyCircuitBreakerEnabled, "false"); err != nil {
		t.Fatalf("关闭熔断器失败：%v", err)
	}
	for range 4 {
		RecordFailure(channelID, keyID, modelName)
	}

	if err := op.SettingSetString(model.SettingKeyCircuitBreakerEnabled, "true"); err != nil {
		t.Fatalf("启用熔断器失败：%v", err)
	}
	RecordFailure(channelID, keyID, modelName)
	if tripped, _ := IsTripped(channelID, keyID, modelName); tripped {
		t.Fatal("关闭期间的失败不应在重新启用后触发熔断")
	}

	for range 4 {
		RecordFailure(channelID, keyID, modelName)
	}
	if tripped, _ := IsTripped(channelID, keyID, modelName); !tripped {
		t.Fatal("启用后的连续失败应触发熔断")
	}

	if err := op.SettingSetString(model.SettingKeyCircuitBreakerEnabled, "false"); err != nil {
		t.Fatalf("再次关闭熔断器失败：%v", err)
	}
	if tripped, _ := IsTripped(channelID, keyID, modelName); tripped {
		t.Fatal("关闭熔断器后不应跳过请求")
	}

	if err := op.SettingSetString(model.SettingKeyCircuitBreakerEnabled, "true"); err != nil {
		t.Fatalf("再次启用熔断器失败：%v", err)
	}
	if tripped, _ := IsTripped(channelID, keyID, modelName); tripped {
		t.Fatal("关闭熔断器应清除已有熔断状态")
	}
}
