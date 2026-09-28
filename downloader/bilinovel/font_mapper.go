package bilinovel

import (
	"fmt"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type glyphMapper struct {
	special  *truetype.Font
	standard *truetype.Font
	lastRune rune
}

func newGlyphMapper(specialData, standardData []byte) (*glyphMapper, error) {
	special, err := truetype.Parse(specialData)
	if err != nil {
		return nil, fmt.Errorf("parse special font: %w", err)
	}
	standard, err := truetype.Parse(standardData)
	if err != nil {
		return nil, fmt.Errorf("parse standard font: %w", err)
	}

	lastRune := rune(0)
	for r := rune(0x10ffff); r >= 0; r-- {
		if standard.Index(r) != 0 {
			lastRune = r
			break
		}
	}
	return &glyphMapper{special: special, standard: standard, lastRune: lastRune}, nil
}

func hasVisibleGlyph(f *truetype.Font, r rune) bool {
	if f.Index(r) == 0 && r != 0 {
		return false
	}
	face := truetype.NewFace(f, &truetype.Options{Size: 12})
	defer face.Close()
	bounds, advance, ok := face.GlyphBounds(r)
	return ok && (!bounds.Empty() || advance != 0) && f.Index(r) != 0
}

func (m *glyphMapper) mapRune(r rune) (rune, bool) {
	if !hasVisibleGlyph(m.special, r) {
		return 0, false
	}
	var specialBuf, standardBuf truetype.GlyphBuf
	if err := specialBuf.Load(m.special, fixed.I(1000), m.special.Index(r), font.HintingNone); err != nil {
		return 0, false
	}

	matches := func(candidate rune) bool {
		if !hasVisibleGlyph(m.standard, candidate) {
			return false
		}
		if err := standardBuf.Load(m.standard, fixed.I(1000), m.standard.Index(candidate), font.HintingNone); err != nil {
			return false
		}
		if len(specialBuf.Ends) != len(standardBuf.Ends) || len(specialBuf.Points) != len(standardBuf.Points) {
			return false
		}
		for i, end := range specialBuf.Ends {
			if end != standardBuf.Ends[i] {
				return false
			}
		}
		for i, point := range specialBuf.Points {
			other := standardBuf.Points[i]
			if absFixed(point.X-other.X) > 10 || absFixed(point.Y-other.Y) > 10 {
				return false
			}
		}
		return true
	}

	if matches(r) {
		return r, true
	}
	for candidate := rune(0); candidate <= m.lastRune; candidate++ {
		if candidate != r && matches(candidate) {
			return candidate, true
		}
	}
	return 0, false
}

func absFixed(v fixed.Int26_6) fixed.Int26_6 {
	if v < 0 {
		return -v
	}
	return v
}
