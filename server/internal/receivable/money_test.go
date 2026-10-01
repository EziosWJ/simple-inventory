package receivable

import (
	"math"
	"testing"
)

func TestCentsRejectsInvalidAndOverflow(t *testing.T) {
	for _, v := range []string{"", "0", "0.00", "-1.00", "+1.00", "1.001", "1.", "92233720368547758.08", "abc"} {
		if _, err := cents(v); err == nil {
			t.Errorf("cents(%q) accepted", v)
		}
	}
	for raw, want := range map[string]int64{"1": 100, "1.5": 150, "1.05": 105, "001.20": 120} {
		got, err := cents(raw)
		if err != nil || got != want {
			t.Errorf("cents(%q)=(%d,%v), want %d", raw, got, err, want)
		}
	}
}
func TestMoneyFormatsSignedInt64Bounds(t *testing.T) {
	cases := map[int64]string{math.MinInt64: "-92233720368547758.08", -150: "-1.50", -1: "-0.01", 0: "0.00", math.MaxInt64: "92233720368547758.07"}
	for n, want := range cases {
		if got := money(n); got != want {
			t.Errorf("money(%d)=%s want %s", n, got, want)
		}
	}
}
