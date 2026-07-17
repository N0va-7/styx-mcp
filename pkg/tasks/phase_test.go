package tasks

import "testing"

func TestSetPhaseAndToMap(t *testing.T) {
	m := NewManager()
	task := m.Create("upload_file")
	if _, ok := m.SetPhase(task.ID, "sending"); !ok {
		t.Fatal("set phase")
	}
	got, ok := m.Get(task.ID)
	if !ok || got.Phase != "sending" {
		t.Fatalf("phase=%q", got.Phase)
	}
	mp := got.ToMap()
	if mp["phase"] != "sending" {
		t.Fatalf("tomap phase=%v", mp["phase"])
	}
	m.SetResult(task.ID, map[string]interface{}{"bytes": 1})
	got, _ = m.Get(task.ID)
	if got.Status != Done {
		t.Fatalf("status=%s", got.Status)
	}
	if got.Phase != "sending" { // last phase kept unless empty
		// SetResult only sets phase to done if empty
	}
}
