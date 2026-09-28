package bilinovel

import "testing"

func TestGlyphMapping(t *testing.T) {
	m, err := newGlyphMapper(readTTF, miLantingTTF)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		input rune
		want  rune
		ok    bool
	}{
		{'A', 'A', true},
		{'你', 0, false},
		{'我', 0, false},
		{'。', '。', true},
		{0x10ffff, 0, false},
		{0xe000, 0x72fc, true},
		{0xe001, 0x8845, true},
		{0xe002, 0x653f, true},
	} {
		got, ok := m.mapRune(tc.input)
		if got != tc.want || ok != tc.ok {
			t.Errorf("U+%04X: got U+%04X, %t; want U+%04X, %t", tc.input, got, ok, tc.want, tc.ok)
		}
	}
}
