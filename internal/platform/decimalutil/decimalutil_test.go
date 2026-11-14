package decimalutil

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestDecimalConversion(t *testing.T) {
	orig := decimal.NewFromFloat(3.20)
	num := ToNumeric(orig)
	back := FromNumeric(num)

	if !orig.Equal(back) {
		t.Errorf("expected %s, got %s", orig, back)
	}
}
