package outbound

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

type got struct {
	Method, URL, Body string
	Header            http.Header
}

// receiver is a server that keeps what it was sent and answers with the status in ?status=.
func receiver(t *testing.T, tls bool) (*httptest.Server, func() []got) {
	var mu sync.Mutex
	var list []got
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		list = append(list, got{r.Method, r.URL.String(), string(body), r.Header})
		mu.Unlock()
		switch r.URL.Path {
		case "/moved":
			http.Redirect(w, r, "/here", http.StatusFound)
		case "/away":
			http.Redirect(w, r, "http://other.invalid/", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/slow":
			time.Sleep(300 * time.Millisecond)
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
		case "/denied":
			w.WriteHeader(http.StatusForbidden)
		}
	})
	var srv *httptest.Server
	if tls {
		srv = httptest.NewTLSServer(h)
	} else {
		srv = httptest.NewServer(h)
	}
	t.Cleanup(srv.Close)
	return srv, func() []got {
		mu.Lock()
		defer mu.Unlock()
		return append([]got(nil), list...)
	}
}

func call(t *testing.T, s *Sender, c engine.Call) engine.CallResult {
	t.Helper()
	done := make(chan engine.CallResult, 1)
	if !s.Call(c, func(r engine.CallResult) { done <- r }) {
		t.Fatal("not queued")
	}
	select {
	case r := <-done:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("no result")
	}
	return engine.CallResult{}
}

func hook(w model.Webhook, values map[string]string) engine.Call {
	return engine.Call{Action: model.Action{Kind: model.ActionWebhook, Webhook: &w}, Values: values}
}

// The request as the server sees it: method, address, headers and body with the placeholders
// filled in and escaped for where they stand.
func TestWebhookRequest(t *testing.T) {
	srv, seen := receiver(t, false)
	s := &Sender{UserAgent: "logic-test", Secret: func(name string) (string, bool) { return map[string]string{"token": "s3cr&t\n"}[name], name == "token" }}
	defer s.Close()
	values := map[string]string{"program": `Licht "Flur" & mehr/weniger`, "trigger": "Tür\nauf", "value": "21.5", "time": "2026-11-22T16:30:00+01:00", "var:name": "ä b"}
	res := call(t, s, hook(model.Webhook{Method: "POST", URL: srv.URL + "/in/{var:name}?p={program}&t={time}&k={secret:token}",
		Headers:  []model.Header{{Name: "Authorization", Value: "Bearer {secret:token}"}, {Name: "X-Trigger", Value: "{trigger}"}},
		BodyType: model.BodyJSON, Body: `{"program": "{program}", "value": {value}, "by": "{trigger}", "keep": {"x": "{other}"}}`}, values))
	if res.Err != nil || res.Status != 200 || res.Took <= 0 {
		t.Fatalf("result %+v", res)
	}
	g := seen()[0]
	if g.Method != "POST" || g.URL != "/in/%C3%A4%20b?p=Licht%20%22Flur%22%20%26%20mehr%2Fweniger&t=2026-11-22T16%3A30%3A00%2B01%3A00&k=s3cr%26t%0A" {
		t.Errorf("request %s %s", g.Method, g.URL)
	}
	if g.Body != `{"program": "Licht \"Flur\" `+"\\u"+`0026 mehr/weniger", "value": 21.5, "by": "Tür\nauf", "keep": {"x": "{other}"}}` {
		t.Errorf("body %q", g.Body)
	}
	if g.Header.Get("Authorization") != "Bearer s3cr&t" || g.Header.Get("X-Trigger") != "Tür auf" || g.Header.Get("Content-Type") != "application/json" || g.Header.Get("User-Agent") != "logic-test" {
		t.Errorf("headers %v", g.Header)
	}

	// a form body, a text body, and a Content-Type of one's own
	call(t, s, hook(model.Webhook{Method: "PUT", URL: srv.URL + "/form", BodyType: model.BodyForm, Body: "p={program}&v={value}"}, values))
	call(t, s, hook(model.Webhook{Method: "POST", URL: srv.URL + "/text", BodyType: model.BodyText, Body: "{program}: {value}",
		Headers: []model.Header{{Name: "Content-Type", Value: "text/x-own"}}}, values))
	call(t, s, hook(model.Webhook{Method: "GET", URL: srv.URL + "/get"}, nil))
	all := seen()
	if g := all[1]; g.Method != "PUT" || g.Body != "p=Licht+%22Flur%22+%26+mehr%2Fweniger&v=21.5" || g.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("form %+v", g)
	}
	if g := all[2]; g.Body != `Licht "Flur" & mehr/weniger: 21.5` || g.Header.Get("Content-Type") != "text/x-own" {
		t.Errorf("text %+v", g)
	}
	if g := all[3]; g.Method != "GET" || g.Body != "" || g.Header.Get("Content-Type") != "" {
		t.Errorf("get %+v", g)
	}
}

// What counts as done and what as failed; and no error ever says the address.
func TestWebhookAnswers(t *testing.T) {
	srv, seen := receiver(t, false)
	s := &Sender{Timeout: 150 * time.Millisecond, Secret: func(string) (string, bool) { return "", false }}
	defer s.Close()
	for _, c := range []struct {
		path   string
		status int
		err    string
	}{
		{"/ok", 200, ""},
		{"/fail", 500, "HTTP 500"},
		{"/denied", 403, "HTTP 403"},
		{"/moved", 200, ""},        // a redirect on the same host is followed
		{"/away", 302, "HTTP 302"}, // one to another host is not
		{"/loop", 302, "HTTP 302"},
		{"/slow", 0, "no answer in time"},
		{"/x?k={secret:missing}", 0, "the secret missing is not set"},
	} {
		res := call(t, s, hook(model.Webhook{Method: "GET", URL: srv.URL + c.path}, nil))
		text := ""
		if res.Err != nil {
			text = res.Err.Error()
		}
		if res.Status != c.status || text != c.err {
			t.Errorf("%s: %d %q", c.path, res.Status, text)
		}
	}
	for _, g := range seen() {
		if strings.Contains(g.URL, "missing") {
			t.Error("a request without its secret was sent")
		}
	}
	// 4xx and 5xx are not tried again, nor is a timeout
	n := map[string]int{}
	for _, g := range seen() {
		n[g.URL]++
	}
	if n["/fail"] != 1 || n["/slow"] != 1 || n["/loop"] != maxRedirects+1 {
		t.Errorf("requests %v", n)
	}

	// nobody listens: tried twice, and the error names no address
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := "http://" + l.Addr().String()
	l.Close()
	res := call(t, s, hook(model.Webhook{Method: "GET", URL: closed + "/?key=verysecret"}, nil))
	if res.Err == nil || res.Status != 0 || strings.Contains(res.Err.Error(), "verysecret") || !strings.Contains(res.Err.Error(), "refused") {
		t.Errorf("closed port: %+v", res)
	}
	// an address that is none (only a hand-made call gets here)
	res = call(t, s, hook(model.Webhook{Method: "GET", URL: "http://[::1"}, nil))
	if res.Err == nil {
		t.Error("a broken address")
	}
	res = call(t, s, engine.Call{})
	if res.Err == nil {
		t.Error("an empty call")
	}
}

// A connection that breaks is tried once more.
func TestWebhookRetriesAConnectionError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var conns atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			if conns.Add(1) == 1 {
				c.Close() // the first one is cut
				continue
			}
			go func() {
				buf := make([]byte, 4096)
				c.Read(buf)
				c.Write([]byte("HTTP/1.1 204 No Content\r\nConnection: close\r\n\r\n"))
				c.Close()
			}()
		}
	}()
	s := &Sender{}
	defer s.Close()
	res := call(t, s, hook(model.Webhook{Method: "POST", URL: "http://" + l.Addr().String() + "/", BodyType: model.BodyText, Body: "x"}, nil))
	if res.Err != nil || res.Status != 204 || conns.Load() != 2 {
		t.Errorf("result %+v after %d connections", res, conns.Load())
	}
}

// A certificate nobody signed is refused, unless the webhook says not to look.
func TestWebhookCertificates(t *testing.T) {
	srv, _ := receiver(t, true)
	s := &Sender{}
	defer s.Close()
	res := call(t, s, hook(model.Webhook{Method: "GET", URL: srv.URL + "/"}, nil))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "certificate") {
		t.Errorf("verified: %+v", res)
	}
	res = call(t, s, hook(model.Webhook{Method: "GET", URL: srv.URL + "/", Insecure: true}, nil))
	if res.Err != nil || res.Status != 200 {
		t.Errorf("not verified: %+v", res)
	}
}

// The queue is bounded: what does not fit is refused at once; Close cuts what is under way.
func TestQueueAndClose(t *testing.T) {
	hold := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-hold }))
	defer srv.Close()
	defer close(hold)
	s := &Sender{Workers: 1, Queue: 2}
	results := make(chan engine.CallResult, 8)
	c := hook(model.Webhook{Method: "GET", URL: srv.URL}, nil)
	queued := 0
	for i := 0; i < 6; i++ {
		if s.Call(c, func(r engine.CallResult) { results <- r }) {
			queued++
		}
		time.Sleep(20 * time.Millisecond)
	}
	if queued != 3 { // one under way, two waiting
		t.Errorf("%d queued", queued)
	}
	s.Close()
	select {
	case r := <-results:
		if r.Err == nil {
			t.Errorf("the cut request: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not end the request")
	}
}

// Wake-on-LAN: three magic packets to the address' port.
func TestWake(t *testing.T) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	s := &Sender{WakePort: conn.LocalAddr().(*net.UDPAddr).Port}
	defer s.Close()
	res := call(t, s, engine.Call{Action: model.Action{Kind: model.ActionWake, Wake: &model.Wake{MAC: "AA:BB:CC:00:11:22", Broadcast: "127.0.0.1"}}})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	want := append(bytes.Repeat([]byte{0xff}, 6), bytes.Repeat([]byte{0xaa, 0xbb, 0xcc, 0x00, 0x11, 0x22}, 16)...)
	buf := make([]byte, 200)
	for i := 0; i < 3; i++ {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := conn.Read(buf)
		if err != nil || !bytes.Equal(buf[:n], want) {
			t.Fatalf("packet %d: %v % x", i, err, buf[:n])
		}
	}
	// nobody there is no error: a machine that is off does not answer
	conn.Close()
	if res := call(t, s, engine.Call{Action: model.Action{Kind: model.ActionWake, Wake: &model.Wake{MAC: "AA:BB:CC:00:11:22", Broadcast: "127.0.0.1"}}}); res.Err != nil {
		t.Errorf("nobody listens: %v", res.Err)
	}
	if res := call(t, s, engine.Call{Action: model.Action{Kind: model.ActionWake, Wake: &model.Wake{MAC: "nope"}}}); res.Err == nil {
		t.Error("an address that is none")
	}
	// the local network's broadcast address is the default (sending may be refused where the
	// test runs: only that it is tried)
	call(t, s, engine.Call{Action: model.Action{Kind: model.ActionWake, Wake: &model.Wake{MAC: "AA:BB:CC:00:11:22"}}})
}
