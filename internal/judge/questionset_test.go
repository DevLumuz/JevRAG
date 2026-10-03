package judge_test

import (
	"os"
	"path/filepath"
	"testing"

	"jev/internal/judge"
)

func writeSet(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "set.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadQuestionSet(t *testing.T) {
	p := writeSet(t, `{"name":"s","questions":{
		"b":{"type":"noul","instructions":"The passage answers the query.","criteria":{"true":"yes","false":"no"}},
		"a":{"type":"choice","instructions":"Role?","criteria":{"x":"..","y":null}}}}`)
	qs, err := judge.LoadQuestionSet(p)
	if err != nil {
		t.Fatal(err)
	}
	if ids := qs.IDs(); len(ids) != 2 || ids[0] != "a" {
		t.Errorf("IDs = %v", ids)
	}
}

func TestLoadQuestionSet_Invalid(t *testing.T) {
	for name, body := range map[string]string{
		"no name":         `{"questions":{"a":{"type":"noul","instructions":"x"}}}`,
		"bad type":        `{"name":"s","questions":{"a":{"type":"maybe","instructions":"x"}}}`,
		"choice no crit":  `{"name":"s","questions":{"a":{"type":"choice","instructions":"x"}}}`,
		"no instructions": `{"name":"s","questions":{"a":{"type":"noul"}}}`,
	} {
		if _, err := judge.LoadQuestionSet(writeSet(t, body)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}
