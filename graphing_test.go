package main

import (
	"os"
	"testing"
)

func TestRenderChartForDate(t *testing.T) {
	month := "September"
	day := "26"
	path, err := RenderChartForDate(month, day)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	// Clean up
	err = os.Remove(path)
	if err != nil {
		t.Errorf("failed to remove %s: %v", path, err)
	}
	t.Logf("Generated: %s", path)
}