// Package notebook holds the pieces of the evidence notebook (plan.md §22):
// splitting passages into sentences, the facts the notebook carries between
// exploration rounds, and the "silver" facts used to test it.
package notebook

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// abbreviations that end with a period but do not end a sentence.
var abbreviations = map[string]bool{
	"mr": true, "mrs": true, "ms": true, "dr": true, "st": true, "jr": true, "sr": true,
	"vs": true, "no": true, "art": true, "inc": true, "ltd": true, "co": true, "corp": true,
	"e.g": true, "i.e": true, "etc": true, "u.s": true, "u.k": true, "mt": true, "ft": true,
	"gen": true, "col": true, "lt": true, "sgt": true, "capt": true, "gov": true, "sen": true,
	"rep": true, "rev": true, "prof": true, "fig": true, "vol": true, "approx": true,
	// Spanish
	"núm": true, "fracc": true, "lic": true, "sra": true, "dra": true, "pág": true, "ing": true,
	"cía": true, "depto": true, "av": true, "fr": true, "párr": true,
}

// SplitSentences splits text into sentences. A sentence ends at '.', '!' or
// '?' (plus closing quotes/brackets) followed by whitespace and an uppercase
// letter, digit or opening quote — unless the period closes a known
// abbreviation or a single-letter initial ("J. R. Smith"). Language-agnostic
// enough for Latin-script text; other scripts fall back to whole lines.
func SplitSentences(text string) []string {
	var out []string
	start := 0
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\n' {
			if s := strings.TrimSpace(string(rs[start:i])); s != "" {
				out = append(out, s)
			}
			start = i + 1
			continue
		}
		if r != '.' && r != '!' && r != '?' {
			continue
		}
		end := i + 1
		for end < len(rs) && strings.ContainsRune(`"')]”’`, rs[end]) {
			end++
		}
		if end >= len(rs) || !unicode.IsSpace(rs[end]) {
			continue
		}
		next := end
		for next < len(rs) && unicode.IsSpace(rs[next]) {
			next++
		}
		if next >= len(rs) {
			continue
		}
		if n := rs[next]; !unicode.IsUpper(n) && !unicode.IsDigit(n) && !strings.ContainsRune(`"'“‘(¿¡«`, n) {
			continue
		}
		if r == '.' && isAbbreviation(rs[start:i]) {
			continue
		}
		if s := strings.TrimSpace(string(rs[start:end])); s != "" {
			out = append(out, s)
		}
		start = end
	}
	if s := strings.TrimSpace(string(rs[start:])); s != "" {
		out = append(out, s)
	}
	return out
}

// isAbbreviation reports whether the word right before a period is an
// abbreviation or an initial.
func isAbbreviation(before []rune) bool {
	j := len(before)
	for j > 0 && !unicode.IsSpace(before[j-1]) && before[j-1] != '(' {
		j--
	}
	w := strings.ToLower(string(before[j:]))
	if utf8.RuneCountInString(w) == 1 && unicode.IsLetter([]rune(w)[0]) {
		return true // initial
	}
	return abbreviations[w]
}
