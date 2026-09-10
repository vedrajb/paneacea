package model

import (
	"math"
	"testing"
)

func TestRecursiveCloseCollapsesOnlyOwnedBranch(t *testing.T) {
	tree := &Layout{Orientation: "vertical", Ratio: .6, First: &Layout{PaneID: "a"}, Second: &Layout{Orientation: "horizontal", Ratio: .4, First: &Layout{PaneID: "b"}, Second: &Layout{PaneID: "c"}}}
	tree = tree.Remove("b")
	if tree.First.PaneID != "a" || tree.Second.PaneID != "c" || tree.Ratio != .6 {
		t.Fatalf("unexpected tree: %+v", tree)
	}
	tree = tree.Remove("a")
	if tree.PaneID != "c" {
		t.Fatal("remaining leaf not promoted")
	}
	if tree.Remove("c") != nil {
		t.Fatal("last pane not removed")
	}
}
func TestSplitPathAndRatioValidation(t *testing.T) {
	tree := &Layout{Orientation: "vertical", Ratio: .5, First: &Layout{PaneID: "a"}, Second: &Layout{PaneID: "b"}}
	if _, err := tree.At([]int{2}); err == nil {
		t.Fatal("invalid path accepted")
	}
	if _, err := tree.At([]int{0}); err == nil {
		t.Fatal("leaf accepted as split")
	}
	for _, r := range []float64{0, 1, math.NaN(), math.Inf(1)} {
		if ValidRatio(r) {
			t.Fatalf("invalid ratio accepted %v", r)
		}
	}
}
