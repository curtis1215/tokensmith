package tui

import (
	"strings"
	"testing"

	"tokensmith/internal/model"
)

func TestOverviewShowsKPIsAndTraining(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = true
	m.state.Training = model.TrainingJob{Gen: 4, WorkRemaining: 500}
	m.state.Models = []model.Model{{Online: true, Users: 1000, Price: 12}}
	m.page = PageOverview
	v := renderOverview(m)
	for _, want := range []string{"估值", "總用戶", "月營收", "排名", "訓練 / 發佈", "Gen4", "里程碑"} {
		if !strings.Contains(v, want) {
			t.Errorf("overview missing %q:\n%s", want, v)
		}
	}
}

func TestOverviewNoTrainingHint(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = false
	if !strings.Contains(renderOverview(m), "無進行中訓練") {
		t.Errorf("expected idle-training hint")
	}
}

func TestOverviewShowsDrafts(t *testing.T) {
	m := testModel(t)
	m.state.HasTraining = false
	// Add a draft model
	m.state.Models = []model.Model{{Online: false, Users: 0, Price: 12}}
	v := renderOverview(m)
	if !strings.Contains(v, "待發佈") {
		t.Errorf("expected draft warning '待發佈', got:\n%s", v)
	}
}

func TestOverviewShowsShare(t *testing.T) {
	m := testModel(t)
	m.state.Models = []model.Model{{Online: true, Segment: model.SegConsumer, Quality: [model.NumQualityDims]float64{10, 0, 0, 0}, Name: "MyModel"}}
	v := renderOverview(m)
	if !strings.Contains(v, "市佔") {
		t.Errorf("expected '市佔', got:\n%s", v)
	}
}
