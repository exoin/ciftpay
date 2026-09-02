package fiscal

import (
	"fmt"
	"math/big"
)

// TaxCategory is the KRA eTIMS tax type letter.
type TaxCategory string

// KRA tax categories. Rates are basis points of the VAT-exclusive value.
const (
	TaxExempt    TaxCategory = "A" // exempt supplies
	TaxStandard  TaxCategory = "B" // 16 %
	TaxZeroRated TaxCategory = "C" // 0 % exports and zero-rated supplies
	TaxNonVAT    TaxCategory = "D" // seller not VAT-registered
	TaxReduced   TaxCategory = "E" // 8 % (fuel etc.)
)

var rateBP = map[TaxCategory]int64{
	TaxExempt:    0,
	TaxStandard:  1600,
	TaxZeroRated: 0,
	TaxNonVAT:    0,
	TaxReduced:   800,
}

// Valid reports whether c is a known category.
func (c TaxCategory) Valid() bool {
	_, ok := rateBP[c]
	return ok
}

// RateBP returns the VAT rate in basis points (1600 = 16 %).
func (c TaxCategory) RateBP() int64 { return rateBP[c] }

// ParseTaxCategory validates a category letter.
func ParseTaxCategory(s string) (TaxCategory, error) {
	c := TaxCategory(s)
	if !c.Valid() {
		return "", fmt.Errorf("fiscal: unknown tax category %q", s)
	}
	return c, nil
}

// DefaultTaxCategory is the category applied to cash sales when the item has
// none: B for VAT-registered sellers, D otherwise.
func DefaultTaxCategory(vatRegistered bool) TaxCategory {
	if vatRegistered {
		return TaxStandard
	}
	return TaxNonVAT
}

// LineTax extracts the VAT contained in a VAT-inclusive line total:
// tax = total × rate / (10000 + rate), rounded half to even.
// Prices in Kenya are quoted VAT-inclusive (docs/data-model.md §5).
func LineTax(lineTotalCents int64, rateBP int64) int64 {
	if rateBP == 0 || lineTotalCents == 0 {
		return 0
	}
	num := new(big.Int).Mul(big.NewInt(lineTotalCents), big.NewInt(rateBP))
	den := big.NewInt(10000 + rateBP)
	return roundHalfEven(num, den)
}

// roundHalfEven divides num by den with banker's rounding.
func roundHalfEven(num, den *big.Int) int64 {
	q, r := new(big.Int).QuoRem(num, den, new(big.Int))
	twice := new(big.Int).Mul(new(big.Int).Abs(r), big.NewInt(2))
	cmp := twice.Cmp(den)
	sign := int64(1)
	if num.Sign() < 0 {
		sign = -1
	}
	switch {
	case cmp > 0:
		q.Add(q, big.NewInt(sign))
	case cmp == 0 && q.Bit(0) == 1:
		q.Add(q, big.NewInt(sign))
	}
	return q.Int64()
}

// Totals sums lines into invoice totals without re-rounding.
func Totals(lines []Line) (subtotal, tax, total int64) {
	for _, l := range lines {
		total += l.LineTotalCents
		tax += l.LineTaxCents
	}
	return total - tax, tax, total
}
