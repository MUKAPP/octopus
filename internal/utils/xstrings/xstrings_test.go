package xstrings

import (
	"reflect"
	"testing"
)

func TestSplitCompactPreservesSplitItems(t *testing.T) {
	got := SplitCompact(",", "", "a, b", ",")
	want := []string{"a", " b", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitCompact returned %#v, want %#v", got, want)
	}
}
