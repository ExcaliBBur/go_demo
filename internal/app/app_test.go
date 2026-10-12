package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"godemo/internal/store"
)

func newStore() *store.Store { return store.New(500, 0, 64) }

func get(h http.Handler, url string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	return rec
}

func TestSameAnswers(t *testing.T) {
	db := newStore()
	fast := NewFast(db)
	defer fast.Close()
	naive, quick := NewNaive(db).Routes(), fast.Routes()

	for _, url := range []string{"/products/42", "/report", "/hash?data=go"} {
		a, b := get(naive, url), get(quick, url)
		if a.Code != http.StatusOK || b.Code != http.StatusOK {
			t.Fatalf("%s: status %d and %d", url, a.Code, b.Code)
		}
		if url == "/hash?data=go" {
			if a.Body.String() != b.Body.String() {
				t.Fatalf("%s: different hashes", url)
			}
			continue
		}
		var x, y any
		if err := json.Unmarshal(a.Body.Bytes(), &x); err != nil {
			t.Fatalf("%s naive: %v", url, err)
		}
		if err := json.Unmarshal(b.Body.Bytes(), &y); err != nil {
			t.Fatalf("%s fast: %v", url, err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Fatalf("%s: answers differ", url)
		}
	}
}

func TestNotFound(t *testing.T) {
	db := newStore()
	fast := NewFast(db)
	defer fast.Close()
	for name, h := range map[string]http.Handler{"naive": NewNaive(db).Routes(), "fast": fast.Routes()} {
		if code := get(h, "/products/100500").Code; code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", name, code)
		}
	}
}

func benchReport(b *testing.B, h http.Handler) {
	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(httptest.NewRecorder(), req)
	}
}

func BenchmarkReportNaive(b *testing.B) {
	benchReport(b, NewNaive(newStore()).Routes())
}

func BenchmarkReportFast(b *testing.B) {
	fast := NewFast(newStore())
	defer fast.Close()
	benchReport(b, fast.Routes())
}
