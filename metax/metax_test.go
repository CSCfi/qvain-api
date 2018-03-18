
package metax

import (
	"testing"
	"time"
	"log"
	"net/http"
	"net/http/httptest"
	
	"reflect"
	"runtime"
	"strings"
)

const (
	// these should time-out
	MANY_TIMES = 100
	LONG_TIME = MANY_TIMES*time.Second
	// just less than time-out
	FEW_TIMES = 9
	SHORT_TIME = FEW_TIMES*time.Second
)

func funcName(f interface{}) string {
	p := reflect.ValueOf(f).Pointer()
	rf := runtime.FuncForPC(p)
	name := rf.Name()
	if name == "" {
		return name
	}
	return name[strings.LastIndexByte(name[:len(name)-1], '.')+1:]
}

var testMux *http.ServeMux
var testServer *httptest.Server
var handlerList = []struct {
	name  string
	url   string
	hfunc func(http.ResponseWriter, *http.Request)
} {
	{"no-response", "/rest/datasets/no-response", NoResponse},
	{"not-json", "/rest/datasets/doge", NotJson},
	{"wrong-content-type", "/rest/datasets/wrong-content-type", WrongContentType},
	{"internal-server-error", "/rest/datasets/internal-server-error", InternalServerError},
	{"slowish-response", "/rest/datasets/slowish-response", SlowishResponse},
	{"slow-header", "/rest/datasets/slow-header", SlowHeader},
	{"slow-body", "/rest/datasets/slow-body", SlowBody},
	{"dripping-body", "/rest/datasets/dripping-body", DrippingBody},
}


//func Datasets(fn func(http.ResponseWriter, *http.Request)) http.Handler {
func Datasets() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatch := r.Header.Get("X-Test")
		if dispatch == "" {
			http.Error(w, "unknown handler test", http.StatusBadRequest)
		}
		for _, h := range handlerList {
			if h.name == dispatch {
				h.hfunc(w, r)
				return
			}
		}
		http.Error(w, "unknown handler test", http.StatusBadRequest)
		return
	})
}


func NoResponse(w http.ResponseWriter, r *http.Request) {
	// we can't close the connection from here, so just silently sleep
	//time.Sleep(LONG_TIME)
}

func NotJson(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	w.Write([]byte("wow  such records  much data  so meta"))
}

func WrongContentType(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "no-sniff")
	
	w.Write([]byte("<html><body>Nobody expects the Spanish Inquisition!</body></html>"))
}

func InternalServerError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("server made boo-boo"))
}

func SlowishResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	w.Write([]byte("slowish response"))
	for i := 0; i < FEW_TIMES; i++ {
		w.Write([]byte{'.'})
		time.Sleep(1*time.Second)
	}
	w.Write([]byte("done!"))
}	

func StallingResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	w.Write([]byte("stalling..."))
	time.Sleep(SHORT_TIME)
	w.Write([]byte("... response"))
}	

func SlowHeader(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	time.Sleep(LONG_TIME)
	w.WriteHeader(http.StatusOK)
	
	w.Write([]byte("slow header"))
}

func SlowBody(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	w.Write([]byte("slow..."))
	time.Sleep(LONG_TIME)
	w.Write([]byte("... body"))
}

func DrippingBody(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	for i := 0; i < MANY_TIMES; i++ {
		w.Write([]byte{'.'})
		time.Sleep(1*time.Second)
	}
}
	

func makeTestMux() *http.ServeMux {
	mux := http.NewServeMux()
	
	for _, h := range handlerList {
		//log.Println("adding handler:", h.name, funcName(h.hfunc))
		mux.HandleFunc(h.url, h.hfunc)
	}
	
	return mux
}


func init() {
	testMux = makeTestMux()
	log.Println("added testserver")
}


// stolen from https://github.com/cypriss/golang-mux-benchmark/
func testRequest(method, path string) (*httptest.ResponseRecorder, *http.Request) {
	request, _ := http.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()
	
	return recorder, request
}


func TestHandlers(t *testing.T) {
	fakeMetax := httptest.NewServer(testMux)
	defer fakeMetax.Close()
	
	log.Println("running fake Metax server on:", fakeMetax.URL)
	
	//url := strings.TrimPrefix(fakeMetax.URL, "http://")
	
	//api := NewMetaxService(url, DisableHttps)
	
	for _, h := range handlerList {
		h := h // capture for parallel tests
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			log.Println("running test handler:", h.name)
			
			t.Log("running test for handler", h.name)
			// response is a ResponseRecorder
			rr, req := testRequest("GET", h.url)
			
			testMux.ServeHTTP(rr, req)
			log.Println("Status:", rr.Code)
			if rr.Code != 200 {
				t.Errorf("(%s) request failed with code: %d", h.name, rr.Code)
				t.FailNow()
			}
			
			if rr.Body.Len() < 1 {
				t.Fatalf("(%s) no response body: (len=%d)\n", h.name, rr.Body.Len())
			}
			/*
			if w.Body.String() != `123` {
				t.Errorf("(%s) unexpected response: %s", h.name, w.Body.String())
			}
			*/
		})
	}
}


/*
func BenchmarkUrl(b *testing.B) {
	w, r := testRequest("GET", "/url")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		testMux.ServeHTTP(w, r)
	}
}
*/
