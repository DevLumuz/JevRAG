package notebook_test

import (
	"reflect"
	"testing"

	"jev/internal/notebook"
)

func TestSplitSentences(t *testing.T) {
	got := notebook.SplitSentences(`J. R. R. Tolkien wrote it in 1937. Dr. Smith said "No." He left! Was it 3.5 km? 1990 was a year.`)
	want := []string{
		"J. R. R. Tolkien wrote it in 1937.",
		`Dr. Smith said "No."`,
		"He left!",
		"Was it 3.5 km?",
		"1990 was a year.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestSilverSentence(t *testing.T) {
	ss := []string{
		"The Latin language was spoken in Rome.",
		"The surname Sylvester comes from the Latin word silvester.",
		"It is common in England.",
	}
	i, k := notebook.SilverSentence(ss, "from the Latin", "where does the last name sylvester come from")
	if i != 1 || k != notebook.SilverExact {
		t.Errorf("got %d %v, want 1 exact", i, k)
	}
	i, k = notebook.SilverSentence(ss, "Medieval Latin", "what was it later known as")
	if k != notebook.SilverPartial || i != 0 {
		t.Errorf("got %d %v, want 0 partial", i, k)
	}
	if _, k := notebook.SilverSentence(ss, "Zanzibar", "unrelated"); k != notebook.SilverNone {
		t.Errorf("got %v, want none", k)
	}
}

func TestSplitSentencesSpanish(t *testing.T) {
	got := notebook.SplitSentences(`Conforme al art. 47, fracc. III, el patrón puede rescindir. ¿Aplica al Lic. Pérez? Ver pág. 12 del anexo. «Fin».`)
	want := []string{
		"Conforme al art. 47, fracc. III, el patrón puede rescindir.",
		"¿Aplica al Lic. Pérez?",
		"Ver pág. 12 del anexo.",
		"«Fin».",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
