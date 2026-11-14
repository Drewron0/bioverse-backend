package decimalutil

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

// ToNumeric converts a shopspring/decimal.Decimal to a pgtype.Numeric.
func ToNumeric(d decimal.Decimal) pgtype.Numeric {
	var num pgtype.Numeric
	_ = num.Scan(d.String())
	return num
}

// FromNumeric converts a pgtype.Numeric to a shopspring/decimal.Decimal.
func FromNumeric(num pgtype.Numeric) decimal.Decimal {
	if !num.Valid {
		return decimal.Zero
	}
	val, err := num.Value()
	if err != nil || val == nil {
		return decimal.Zero
	}
	s := fmt.Sprintf("%v", val)
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}
