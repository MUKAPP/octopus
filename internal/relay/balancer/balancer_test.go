package balancer

import (
	"testing"

	"github.com/bestruirui/octopus/internal/model"
)

func TestRatePriorityCandidatesSortByRateThenPriority(t *testing.T) {
	items := []model.GroupItem{
		{ID: 1, Priority: 1, RateMultiplier: 1},
		{ID: 2, Priority: 2, RateMultiplier: 0.5},
		{ID: 3, Priority: 1, RateMultiplier: 0.5},
		{ID: 4, Priority: 0, RateMultiplier: 0},
	}

	got := (&RatePriority{}).Candidates(items)
	wantIDs := []int{3, 2, 4, 1}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("第 %d 个候选渠道不符合预期：got %d, want %d", i, got[i].ID, wantID)
		}
	}
	if items[0].ID != 1 {
		t.Fatal("倍率排序不应修改原始候选列表")
	}
}

func TestGetBalancerReturnsRatePriority(t *testing.T) {
	if _, ok := GetBalancer(model.GroupModeRatePriority).(*RatePriority); !ok {
		t.Fatal("倍率优先模式应返回 RatePriority 调度器")
	}
}
