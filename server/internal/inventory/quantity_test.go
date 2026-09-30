package inventory

import "testing"

func TestQuantityBoundaries(t *testing.T) {
	for _, x := range []struct{ input, want string }{{"0.001", "0.001"}, {"-0.001", "-0.001"}, {"+01.230", "1.23"}, {"9223372036854775.807", "9223372036854775.807"}, {"-9223372036854775.808", "-9223372036854775.808"}, {"10.000", "10"}} {
		n, e := parseQuantity(x.input)
		if e != nil {
			t.Fatalf("parse %s: %v", x.input, e)
		}
		if got := quantityText(n); got != x.want {
			t.Errorf("%s: got %s, want %s", x.input, got, x.want)
		}
	}
	for _, input := range []string{"", "0", "-0.000", "1.0000", "1.", ".1", "1e3", " 1", "1 ", "NaN", "--1", "9223372036854775.808", "-9223372036854775.809"} {
		if _, e := parseQuantity(input); e == nil {
			t.Errorf("accepted invalid %q", input)
		}
	}
}
