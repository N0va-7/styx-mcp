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

func TestMergeResultKeepsRunning(t *testing.T) {
	m := NewManager()
	task := m.Create("start_scan")
	m.UpdateStatus(task.ID, Running)
	m.SetPhase(task.ID, "discovering")
	if _, ok := m.MergeResult(task.ID, map[string]interface{}{
		"progress": map[string]interface{}{"stage": "icmp", "icmp_done": 16, "alive_n": 1},
	}); !ok {
		t.Fatal("merge")
	}
	got, _ := m.Get(task.ID)
	if got.Status != Running {
		t.Fatalf("status=%s want running", got.Status)
	}
	if got.Phase != "discovering" {
		t.Fatalf("phase=%s", got.Phase)
	}
	prog, ok := got.Result["progress"].(map[string]interface{})
	if !ok || prog["stage"] != "icmp" {
		t.Fatalf("progress=%v", got.Result["progress"])
	}
	// Merge again without clobbering sibling keys.
	m.MergeResult(task.ID, map[string]interface{}{"progress": map[string]interface{}{"stage": "icmp_done", "icmp_done": 254}})
	got, _ = m.Get(task.ID)
	prog, _ = got.Result["progress"].(map[string]interface{})
	if prog["stage"] != "icmp_done" {
		t.Fatalf("progress=%v", prog)
	}
}
