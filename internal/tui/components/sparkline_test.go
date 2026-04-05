package components

import (
	"testing"
)

func TestSparkline_Empty(t *testing.T) {
	result := Sparkline(nil, 8)
	if len(result) != 8 {
		t.Errorf("expected width 8, got %d", len(result))
	}
}

func TestSparkline_SingleValue(t *testing.T) {
	result := Sparkline([]float64{50.0}, 8)
	if len(result) < 1 {
		t.Error("expected non-empty sparkline")
	}
}

func TestSparkline_Ascending(t *testing.T) {
	values := []float64{0, 25, 50, 75, 100}
	result := Sparkline(values, 5)

	// First char should be lowest block, last should be highest
	runes := []rune(result)
	if len(runes) < 5 {
		t.Fatalf("expected at least 5 runes, got %d", len(runes))
	}

	if runes[0] != '▁' {
		t.Errorf("first char should be ▁, got %c", runes[0])
	}
	if runes[4] != '█' {
		t.Errorf("last char should be █, got %c", runes[4])
	}
}

func TestSparkline_FlatLine(t *testing.T) {
	values := []float64{50, 50, 50, 50}
	result := Sparkline(values, 4)

	// All chars should be the same (lowest block since no range)
	runes := []rune(result)
	for i := 1; i < len(runes); i++ {
		if runes[i] != runes[0] {
			t.Errorf("expected all same char for flat line, got %c at index %d", runes[i], i)
		}
	}
}

func TestSparkline_TruncatesToWidth(t *testing.T) {
	values := make([]float64, 20)
	for i := range values {
		values[i] = float64(i)
	}

	result := Sparkline(values, 5)
	runes := []rune(result)
	// Should only use last 5 values
	if len(runes) < 5 {
		t.Errorf("expected at least 5 runes, got %d", len(runes))
	}
}

func TestSparkline_ZeroWidth(t *testing.T) {
	result := Sparkline([]float64{1, 2, 3}, 0)
	if result != "" {
		t.Errorf("expected empty string for zero width, got %q", result)
	}
}
