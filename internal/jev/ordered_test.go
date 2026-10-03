package jev_test

import (
	"encoding/json"
	"testing"

	"jev/internal/jev"
)

func TestOrdered_KeepsOrder(t *testing.T) {
	q := jev.Question{Type: "choice", Instructions: "x", Criteria: jev.Ordered{
		{Key: "irrelevant", Value: "a"}, {Key: "weak", Value: nil}, {Key: "high", Value: "c"}, {Key: "direct", Value: "d"},
	}}
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"choice","instructions":"x","criteria":{"irrelevant":"a","weak":null,"high":"c","direct":"d"}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

func TestDecodeOrdered_RoundTrip(t *testing.T) {
	src := `{"z":1,"a":{"true":"yes","false":"no"},"m":[{"q":"x","b":true},null]}`
	v, err := jev.DecodeOrdered([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != src {
		t.Errorf("round trip changed order:\n got  %s\n want %s", b, src)
	}
}

func TestChoice_KeepsOptionOrder(t *testing.T) {
	q := jev.Choice("pick", []jev.Option{{Label: "low"}, {Label: "mid", Description: "m"}, {Label: "high"}})
	b, _ := json.Marshal(q)
	want := `{"type":"choice","instructions":"pick","criteria":{"low":null,"mid":"m","high":null}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}
