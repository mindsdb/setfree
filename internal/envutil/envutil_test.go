package envutil

import (
	"reflect"
	"testing"
)

func TestSet_AppendsWhenAbsent(t *testing.T) {
	got := Set([]string{"A=1"}, "B", "2")
	want := []string{"A=1", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSet_ReplacesInPlace(t *testing.T) {
	got := Set([]string{"A=1", "B=old", "C=3"}, "B", "new")
	want := []string{"A=1", "B=new", "C=3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestSet_DoesNotMutateInput(t *testing.T) {
	in := []string{"A=1"}
	_ = Set(in, "A", "2")
	if in[0] != "A=1" {
		t.Errorf("input was mutated: %v", in)
	}
}

func TestSet_CollapsesDuplicates(t *testing.T) {
	got := Set([]string{"A=1", "A=2"}, "A", "3")
	want := []string{"A=3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnset_RemovesKey(t *testing.T) {
	got := Unset([]string{"A=1", "B=2"}, "A")
	want := []string{"B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestUnset_NoOpWhenAbsent(t *testing.T) {
	got := Unset([]string{"B=2"}, "A")
	want := []string{"B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
