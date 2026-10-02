package jev_test

import (
	"context"
	"testing"

	"jev/internal/jev"
)

func TestFakeClient_Choice(t *testing.T) {
	fake := &jev.FakeClient{
		ChoiceResults: map[string]jev.ChoiceResult{
			"Is this article about property law?": {Choice: "High", Confidence: 0.92},
		},
	}

	choice, conf, err := fake.Choice(context.Background(), "Article 40 text...", "Is this article about property law?", []string{"Irrelevant", "Weak", "High", "Direct"})
	if err != nil {
		t.Fatalf("Choice() error: %v", err)
	}
	if choice != "High" {
		t.Errorf("choice = %q, want High", choice)
	}
	if conf != 0.92 {
		t.Errorf("confidence = %f, want 0.92", conf)
	}
}

func TestFakeClient_Noul(t *testing.T) {
	fake := &jev.FakeClient{
		NoulResults: map[string]float64{
			"Is evidence sufficient?": 0.85,
		},
	}

	prob, err := fake.Noul(context.Background(), "some state", "Is evidence sufficient?")
	if err != nil {
		t.Fatalf("Noul() error: %v", err)
	}
	if prob != 0.85 {
		t.Errorf("probability = %f, want 0.85", prob)
	}
}

func TestFakeClient_Score(t *testing.T) {
	fake := &jev.FakeClient{
		ScoreResults: map[string]jev.ScoreResult{
			"How relevant?": {Score: 3.5, Confidence: 0.88},
		},
	}

	score, conf, err := fake.Score(context.Background(), "state", "How relevant?", []string{"1", "2", "3", "4", "5"})
	if err != nil {
		t.Fatalf("Score() error: %v", err)
	}
	if score != 3.5 {
		t.Errorf("score = %f, want 3.5", score)
	}
	if conf != 0.88 {
		t.Errorf("confidence = %f, want 0.88", conf)
	}
}

func TestFakeClient_UnknownQuestion(t *testing.T) {
	fake := &jev.FakeClient{}

	_, _, err := fake.Choice(context.Background(), "state", "unknown question", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error for unknown question, got nil")
	}
}
