package ingest

import "testing"

func TestOrientedDimensions(t *testing.T) {
	tests := []struct {
		name                  string
		orientation           int
		wantWidth, wantHeight int
	}{
		{name: "normal", orientation: 1, wantWidth: 6000, wantHeight: 4000},
		{name: "mirrored", orientation: 2, wantWidth: 6000, wantHeight: 4000},
		{name: "rotated clockwise", orientation: 6, wantWidth: 4000, wantHeight: 6000},
		{name: "rotated counterclockwise", orientation: 8, wantWidth: 4000, wantHeight: 6000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height := orientedDimensions(6000, 4000, test.orientation)
			if width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("orientedDimensions() = %d x %d, want %d x %d", width, height, test.wantWidth, test.wantHeight)
			}
		})
	}
}

func TestSetDisplayDimensionsSetsPortrait(t *testing.T) {
	inspection := Inspection{Metadata: map[string]any{}}
	setDisplayDimensions(&inspection, 4000, 6000)
	if inspection.Dimensions != "4000 × 6000" || inspection.AspectRatio != "portrait" {
		t.Fatalf("unexpected display dimensions: %+v", inspection)
	}
}
