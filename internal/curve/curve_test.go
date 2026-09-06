package curve

import "testing"

func TestGammaMapsEndpointsAndMidpoint(t *testing.T) {
	gamma, err := NewGamma(2)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		input  int
		expect int
	}{
		{input: 0, expect: 0},
		{input: 50, expect: 25},
		{input: 100, expect: 100},
	} {
		actual, err := gamma.Map(test.input)
		if err != nil {
			t.Fatalf("Map(%d): %v", test.input, err)
		}
		if actual != test.expect {
			t.Errorf("Map(%d) = %d, want %d", test.input, actual, test.expect)
		}
	}
}

func TestLookupInterpolatesAndValidates(t *testing.T) {
	lookup, err := NewLookup([]Point{{Input: 0, Output: 0}, {Input: 50, Output: 20}, {Input: 100, Output: 100}})
	if err != nil {
		t.Fatal(err)
	}
	actual, err := lookup.Map(75)
	if err != nil {
		t.Fatal(err)
	}
	if actual != 60 {
		t.Fatalf("Map(75) = %d, want 60", actual)
	}

	if _, err := NewLookup([]Point{{Input: 0, Output: 0}, {Input: 80, Output: 90}}); err == nil {
		t.Fatal("expected endpoint validation error")
	}
}
