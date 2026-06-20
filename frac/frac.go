// Package frac provides rational number types and helpers for the m4bon pipeline.
package frac

// Fraction represents a rational number.
type Fraction struct {
	Num int
	Den int
}

// DPPQ (divisions per quarter note) matches MusicXML convention.
const DPPQ = 480

// TicksPerWholeNote is the number of ticks in a whole note (DPPQ * 4).
const TicksPerWholeNote = DPPQ * 4

// GCD returns the greatest common divisor of a and b.
func GCD(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	for b > 0 {
		a, b = b, a%b
	}
	return a
}

// IsPowerOf2 returns true if n is a power of two.
func IsPowerOf2(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// LowerPowerOf2 returns the largest power of two less than n.
func LowerPowerOf2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p*2 < n {
		p *= 2
	}
	return p
}

// IsStandardDuration returns true if the reduced fraction z/n is a standard
// duration (not a tuplet). Standard means denominator is a power of 2 and
// numerator is 1 or 3.
func IsStandardDuration(z, n int) bool {
	g := GCD(z, n)
	z /= g
	n /= g
	if !IsPowerOf2(n) {
		return false
	}
	return z == 1 || z == 3
}

// LessThan returns true if f < other using cross-multiplication (no float).
func (f Fraction) LessThan(other Fraction) bool {
	return f.Num*other.Den < other.Num*f.Den
}

// Sub returns f - other reduced to lowest terms. Assumes f >= other.
func (f Fraction) Sub(other Fraction) Fraction {
	num := f.Num*other.Den - other.Num*f.Den
	den := f.Den * other.Den
	g := GCD(num, den)
	return Fraction{Num: num / g, Den: den / g}
}

// Add returns f + other reduced to lowest terms.
func (f Fraction) Add(other Fraction) Fraction {
	num := f.Num*other.Den + other.Num*f.Den
	den := f.Den * other.Den
	g := GCD(num, den)
	return Fraction{Num: num / g, Den: den / g}
}

// MulInt multiplies f by num/den and returns the reduced result.
func (f Fraction) MulInt(num, den int) Fraction {
	n := f.Num * num
	d := f.Den * den
	g := GCD(n, d)
	return Fraction{Num: n / g, Den: d / g}
}

// Reduce returns f reduced to lowest terms.
func (f Fraction) Reduce() Fraction {
	g := GCD(f.Num, f.Den)
	return Fraction{Num: f.Num / g, Den: f.Den / g}
}
