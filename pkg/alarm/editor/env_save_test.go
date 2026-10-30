package editor

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveToEnv_UnterminatedValueDoesNotDuplicateFile checks the "save to .env" path of
// the contacts and tags endpoints when the existing CONTACT_LIST / TAG_LIST line has no
// closing single quote anywhere after it. The block end used to stay at -1, and
// lines[endLine+1:] then re-appended the entire file, duplicating every setting
// (including credentials) in the user's .env. Now only the key's own line is replaced.
func TestSaveToEnv_UnterminatedValueDoesNotDuplicateFile(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		path    string
		body    string
		handler func(*Server) http.HandlerFunc
	}{
		{"contacts", "CONTACT_LIST", "/api/contacts/save",
			`{"saveType":"env","contacts":[{"name":"A","email":"a@example.com"}]}`,
			func(s *Server) http.HandlerFunc { return s.handleSaveContacts }},
		{"tags", "TAG_LIST", "/api/tags/save",
			`{"saveType":"env","tags":["outdoor"]}`,
			func(s *Server) http.HandlerFunc { return s.handleSaveTags }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envPath := filepath.Join(t.TempDir(), ".env")
			original := "FIRST=1\n" + tc.key + "=\nLAST=2\n"
			if err := os.WriteFile(envPath, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			s := &Server{envFile: envPath}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
			tc.handler(s)(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}

			data, err := os.ReadFile(envPath)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			for _, line := range []string{"FIRST=1", "LAST=2", tc.key + "="} {
				if n := strings.Count(got, line); n != 1 {
					t.Fatalf("expected %q exactly once, found %d times in:\n%s", line, n, got)
				}
			}
			if !strings.Contains(got, tc.key+"='[") {
				t.Fatalf("expected new %s value in:\n%s", tc.key, got)
			}
		})
	}
}

// TestSaveContactsToEnv_BlockDetection checks which lines the "save to .env" path
// replaces: an unquoted value is one line (a later, unrelated quoted variable must
// survive — it used to be swallowed as the "closing quote"), and a multi-line quoted
// value is replaced up to and including its closing-quote line.
func TestSaveContactsToEnv_BlockDetection(t *testing.T) {
	cases := []struct {
		name     string
		original string
		keep     []string
		gone     []string
	}{
		{"unquoted value then quoted var",
			"A=1\nCONTACT_LIST=[]\nSMS_TEMPLATE='hello'\nZ=2\n",
			[]string{"A=1", "SMS_TEMPLATE='hello'", "Z=2"}, []string{"CONTACT_LIST=[]"}},
		{"multi-line quoted value",
			"A=1\nCONTACT_LIST='[\n  {\"name\":\"Old\",\"email\":\"o@example.com\"}\n]'\nZ=2\n",
			[]string{"A=1", "Z=2"}, []string{"Old"}},
		{"single-line quoted value",
			"A=1\nCONTACT_LIST='[{\"name\":\"Old\",\"email\":\"o@example.com\"}]'\nZ='q'\n",
			[]string{"A=1", "Z='q'"}, []string{"Old"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envPath := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(envPath, []byte(tc.original), 0600); err != nil {
				t.Fatal(err)
			}
			s := &Server{envFile: envPath}
			rec := httptest.NewRecorder()
			body := `{"saveType":"env","contacts":[{"name":"New","email":"n@example.com"}]}`
			s.handleSaveContacts(rec, httptest.NewRequest(http.MethodPost, "/api/contacts/save", bytes.NewBufferString(body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			data, _ := os.ReadFile(envPath)
			got := string(data)
			for _, k := range tc.keep {
				if strings.Count(got, k) != 1 {
					t.Fatalf("expected %q exactly once in:\n%s", k, got)
				}
			}
			for _, g := range tc.gone {
				if strings.Contains(got, g) {
					t.Fatalf("expected %q to be replaced in:\n%s", g, got)
				}
			}
			if strings.Count(got, "CONTACT_LIST=") != 1 || !strings.Contains(got, `"name": "New"`) {
				t.Fatalf("expected exactly one new CONTACT_LIST in:\n%s", got)
			}
		})
	}
}
