// Package outbound carries out what a program does beyond the system: webhooks, Wake-on-LAN
// and commands.
// A few workers behind a bounded queue: the engine hands a call over and goes on, so no slow
// server ever delays a program; a call that finds the queue full is dropped.
package outbound

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// The defaults.
const (
	Workers  = 4
	Queue    = 100
	Timeout  = 10 * time.Second
	WakePort = 9
	// a Wake-on-LAN is three magic packets, this far apart
	wakePackets = 3
	wakeGap     = 100 * time.Millisecond
	// a redirect is followed on the same host only, this many times
	maxRedirects = 5
)

// Sender is the worker pool. The zero value works; set the fields before the first Call.
type Sender struct {
	// Secret answers a secret by its name.
	Secret func(name string) (string, bool)
	// Workers, Queue, Timeout and WakePort replace the defaults when set.
	Workers, Queue int
	Timeout        time.Duration
	WakePort       int
	// UserAgent is the requests' User-Agent.
	UserAgent string
	// Dir is where a command runs (the process' directory when empty); Env is added to the
	// few variables a command gets. CommandWorkers and CommandQueue replace the defaults.
	Dir                          string
	Env                          []string
	CommandWorkers, CommandQueue int

	once     sync.Once
	queue    chan job
	commands chan job
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	verified *http.Client
	insecure *http.Client
}

type job struct {
	call engine.Call
	done func(engine.CallResult)
}

func or[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}

func (s *Sender) start() {
	s.queue = make(chan job, or(s.Queue, Queue))
	s.ctx, s.cancel = context.WithCancel(context.Background())
	client := func(insecure bool) *http.Client {
		return &http.Client{
			Timeout: or(s.Timeout, Timeout),
			Transport: &http.Transport{
				TLSClientConfig:       &tls.Config{InsecureSkipVerify: insecure},
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       30 * time.Second,
				ResponseHeaderTimeout: or(s.Timeout, Timeout),
			},
			// a redirect is followed on the same host; one to another host is not, and its
			// answer is the call's
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) > maxRedirects || req.URL.Host != via[0].URL.Host {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}
	}
	s.verified, s.insecure = client(false), client(true)
	s.commands = make(chan job, or(s.CommandQueue, CommandQueue))
	work := func(n int, queue chan job) {
		for i := 0; i < n; i++ {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				for {
					select {
					case <-s.ctx.Done():
						return
					case j := <-queue:
						j.done(s.do(j.call))
					}
				}
			}()
		}
	}
	work(or(s.Workers, Workers), s.queue)
	work(or(s.CommandWorkers, CommandWorkers), s.commands)
}

// Call queues a webhook, a Wake-on-LAN or a command and returns at once; done is called later, from a
// worker, with what became of it. False when the queue is full: the call is dropped and done
// is never called.
func (s *Sender) Call(c engine.Call, done func(engine.CallResult)) bool {
	s.once.Do(s.start)
	queue := s.queue
	if c.Action.Command != nil {
		queue = s.commands
	}
	select {
	case queue <- job{c, done}:
		return true
	default:
		return false
	}
}

// Close stops the workers: a request under way is cut off, what waits in the queue is dropped.
func (s *Sender) Close() {
	s.once.Do(s.start)
	s.cancel()
	s.wg.Wait()
}

func (s *Sender) do(c engine.Call) engine.CallResult {
	start := time.Now()
	var res engine.CallResult
	switch {
	case c.Action.Command != nil:
		res = s.command(c)
	case c.Action.Webhook != nil:
		res.Status, res.Err = s.webhook(c)
	case c.Action.Wake != nil:
		res.Err = s.wake(c.Action.Wake)
	default:
		res.Err = errors.New("nothing to call")
	}
	res.Took = time.Since(start)
	return res
}

// --- webhooks ---

// escapeURL escapes a placeholder's value for an address: everything but letters, digits and
// - . _ ~ is percent-encoded, so a value never adds a path segment or a parameter.
func escapeURL(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

// escapeJSON escapes a value as the inside of a JSON string.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// escapeHeader keeps a value on its header's line.
func escapeHeader(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == 0 {
			return ' '
		}
		return r
	}, s)
}

var contentTypes = map[string]string{
	model.BodyJSON: "application/json",
	model.BodyForm: "application/x-www-form-urlencoded",
	model.BodyText: "text/plain; charset=utf-8",
}

// build makes the request's parts with the placeholders filled in: a secret that is not set is
// an error, and no request is made.
func (s *Sender) build(c engine.Call) (address string, header http.Header, body string, err error) {
	w := c.Action.Webhook
	fill := func(name string) string {
		if secret, ok := strings.CutPrefix(name, "secret:"); ok {
			v, found := "", false
			if s.Secret != nil {
				v, found = s.Secret(secret)
			}
			if !found && err == nil {
				err = fmt.Errorf("the secret %s is not set", secret)
			}
			return v
		}
		return c.Values[name]
	}
	address = model.Expand(w.URL, fill, escapeURL)
	header = http.Header{}
	if s.UserAgent != "" {
		header.Set("User-Agent", s.UserAgent)
	}
	if w.Body != "" {
		header.Set("Content-Type", contentTypes[w.BodyType])
		escape := map[string]func(string) string{model.BodyJSON: escapeJSON, model.BodyForm: url.QueryEscape, model.BodyText: func(s string) string { return s }}[w.BodyType]
		body = model.Expand(w.Body, fill, escape)
	}
	for _, h := range w.Headers {
		header.Set(h.Name, model.Expand(h.Value, fill, escapeHeader))
	}
	return address, header, body, err
}

// plain answers an error's text without the address (which may hold a secret).
func plain(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return errors.New("no answer in time")
		}
		return ue.Err
	}
	return err
}

// webhook sends the request: the status code is the answer, the body is never read. A
// connection that fails is tried once more; an answer - whatever it says - and a timeout are not.
func (s *Sender) webhook(c engine.Call) (int, error) {
	w := c.Action.Webhook
	address, header, body, err := s.build(c)
	if err != nil {
		return 0, err
	}
	client := s.verified
	if w.Insecure {
		client = s.insecure
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(s.ctx, w.Method, address, strings.NewReader(body))
		if err != nil {
			return 0, plain(err)
		}
		req.Header = header.Clone()
		res, err := client.Do(req)
		if err != nil {
			var ne net.Error
			if attempt == 0 && s.ctx.Err() == nil && !(errors.As(err, &ne) && ne.Timeout()) {
				continue
			}
			return 0, plain(err)
		}
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode > 299 {
			return res.StatusCode, errors.New("HTTP " + strconv.Itoa(res.StatusCode))
		}
		return res.StatusCode, nil
	}
}

// --- Wake-on-LAN ---

// MagicPacket is what wakes a machine: six bytes FF and its hardware address sixteen times.
func MagicPacket(mac string) ([]byte, error) {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) != 6 {
		return nil, fmt.Errorf("%q is not a hardware address", mac)
	}
	p := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	for i := 0; i < 16; i++ {
		p = append(p, hw...)
	}
	return p, nil
}

func (s *Sender) wake(w *model.Wake) error {
	packet, err := MagicPacket(w.MAC)
	if err != nil {
		return err
	}
	to := &net.UDPAddr{IP: net.ParseIP(or(w.Broadcast, "255.255.255.255")), Port: or(s.WakePort, WakePort)}
	// a socket that is not connected: nothing a machine that is off (not) answers comes back
	// as an error of the next packet
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	for i := 0; i < wakePackets; i++ {
		if i > 0 {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			case <-time.After(wakeGap):
			}
		}
		if _, err := conn.WriteToUDP(packet, to); err != nil {
			return err
		}
	}
	return nil
}
