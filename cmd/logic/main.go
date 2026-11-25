// logic is the lite-addon-logic daemon: CCU-style programs and system variables for openccu-lite.
// It serves its page and API on the loopback behind lighttpd's proxy of /addons/logic/ and, on a
// second loopback port, the interface process "Logic" that lite-rpc offers like the radios'
// interfaces. openccu-lite only; no listening port beyond the loopback.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/addonkit"
	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/httpapi"
	"github.com/openccu-lite/lite-addon-logic/internal/importer"
	"github.com/openccu-lite/lite-addon-logic/internal/outbound"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
	"github.com/openccu-lite/lite-addon-logic/internal/rpciface"
	"github.com/openccu-lite/lite-addon-logic/internal/service"
	"github.com/openccu-lite/lite-addon-logic/internal/system"
)

// version is set by the build (-ldflags "-X main.version=…").
var version = "dev"

type options struct {
	addonDir, confDir, id string
	port, rpcPort         int
	base, basePath, www   string
	tokenFile             string
	noSession             bool
	devClock              string
	devWakePort           int
}

func main() {
	log.SetFlags(0) // the journal adds the time
	if len(os.Args) > 1 && os.Args[1] == "anonymise" {
		os.Exit(anonymise(os.Args[2:], os.Stdout, os.Stderr))
	}
	var o options
	flag.StringVar(&o.addonDir, "addon-dir", "/usr/local/addons/logic", "the addon's directory (app/, var/)")
	flag.StringVar(&o.confDir, "conf-dir", "/usr/local/etc/config/addons/logic", "the data directory (tree/: programs and variables as files; state/: values; mode 0700)")
	flag.StringVar(&o.id, "id", "logic", "the addon id (token file, base path)")
	flag.IntVar(&o.port, "port", 8301, "the loopback port lighttpd proxies to")
	flag.IntVar(&o.rpcPort, "rpc-port", 31310, "the loopback port of the interface process \"Logic\" (the manifest's runtime.rpc_interface)")
	flag.StringVar(&o.base, "system", "http://127.0.0.1", "openccu-lite's URL")
	flag.StringVar(&o.basePath, "base-path", "", "the path the page is served at (/addons/<id> by default)")
	flag.StringVar(&o.www, "www", "", "the built page (<addon-dir>/app by default)")
	flag.StringVar(&o.tokenFile, "token-file", "", "a token file for the system's API (the addon's own token by default)")
	flag.BoolVar(&o.noSession, "no-session-check", false, "serve the page without the session check (development only)")
	flag.StringVar(&o.devClock, "dev-clock", "", "development only: run on a clock that stands still at this time (RFC 3339) and is moved through /api/v1/dev/clock")
	flag.IntVar(&o.devWakePort, "dev-wake-port", 0, "development only: the UDP port Wake-on-LAN packets go to (9 by default)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if o.basePath == "" {
		o.basePath = "/addons/" + o.id
	}
	if o.www == "" {
		o.www = filepath.Join(o.addonDir, "app")
	}
	if err := run(o); err != nil {
		log.Fatalf("logic: %v", err)
	}
}

func bootID() string {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// The daemon's memory on a small system: the Go runtime collects when the heap has doubled
// since the last collection, so a process holds up to twice what it uses. Here a collection
// comes when the heap has grown by half - measured on a Raspberry Pi 4 with 300 programs and
// 800 variables under load: 17 MB held where it was 22, for a seventh more processor time.
// (A memory limit close to what is in use was tried and is not set: the collector then runs
// without end - four to six times the processor time. Handing free memory back every minute
// changed nothing.) GOGC in the environment takes precedence, as for every Go program.
const memoryGC = 50

func tuneMemory() {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(memoryGC)
	}
}

func run(o options) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	tuneMemory()
	logf := func(format string, args ...any) { log.Printf("logic: "+format, args...) }

	// the system: device writes go out through lite-rpc, the datapoints' values come in from its
	// state store and event stream, the devices' names from its metadata
	var token addonkit.TokenSource = addonkit.AddonToken(o.id)
	if o.tokenFile != "" {
		token = &addonkit.FileToken{Path: o.tokenFile}
	}
	lite := litesource.New(o.base, token)
	lite.Log = logf
	writer := &system.Writer{Lite: lite}
	names := &system.Names{Lite: lite}
	// the clock: the system's - whose time modules wait until it is set - or, for the page's
	// tests, one that stands still until it is moved
	// webhooks and Wake-on-LAN go out through a few workers of their own; the secrets a
	// webhook names are the service's
	var svc *service.Service
	// a command runs in the addon's var/ directory
	varDir := filepath.Join(o.addonDir, "var")
	_ = os.MkdirAll(varDir, 0o755)
	sender := &outbound.Sender{UserAgent: "lite-addon-logic/" + version, WakePort: o.devWakePort, Secret: func(name string) (string, bool) { return svc.Secret(name) }, Dir: varDir}
	so := service.Options{Dir: o.confDir, Writer: writer, Caller: sender, Pacer: writer.SetGap, Names: names.Name, Log: logf, ClockReady: clockSynchronised}
	var devClock *engine.FakeClock
	if o.devClock != "" {
		at, err := time.Parse(time.RFC3339, o.devClock)
		if err != nil {
			return fmt.Errorf("-dev-clock: %v", err)
		}
		devClock = engine.NewFakeClock(at.In(time.Local))
		so.Clock, so.ClockReady = devClock, nil
		logf("DEVELOPMENT CLOCK: the time stands at %s until it is moved", at.Format(time.RFC3339))
	}
	var err error
	if svc, err = service.Open(so); err != nil {
		return err
	}
	feed := &system.Feed{Lite: lite, Sink: svc.Engine(), Own: rpciface.Interface, Log: logf}
	svc.OnChange(feed.Kick)
	go feed.Run(ctx)
	// the counters the device helpers follow: their maxima come from the devices' descriptions
	maxima := &system.Maxima{Params: &system.Catalog{Lite: lite, Own: rpciface.Interface}, Sink: svc.Engine(), Log: logf}
	svc.OnChange(maxima.Kick)
	go maxima.Run(ctx)
	boot := bootID()
	first := svc.SystemStart(boot)
	if first {
		logf("the first start after a boot of the system")
	}
	// once per boot of the system the programs are evaluated as the CCU does at its start -
	// when the clock is set and every device datapoint these programs look at has a value (right
	// after a boot the radio interfaces are still starting), or after five minutes
	go svc.StartEvaluation(ctx, boot, first, func() bool { return svc.Engine().ClockSet() && len(svc.Engine().StartUnknown()) == 0 }, 5*time.Minute, time.Second)
	// where the sun stands (the system values sun-azimuth and sun-elevation), every five minutes
	go func() {
		tick := time.NewTicker(5 * time.Minute)
		defer tick.Stop()
		for {
			svc.Engine().UpdateSun()
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
		}
	}()
	// the state is written when it changed, at most every 30 s, and at the stop; the delayed
	// activities go into a small file of their own five seconds after they changed (a delay
	// survives a power cut, and a system full of delays does not write its whole state all the time)
	saved := make(chan struct{})
	go func() {
		defer close(saved)
		svc.RunSaver(ctx, service.StateEvery, service.PendingEvery)
	}()

	// the interface process "Logic": variables and programs as devices, on its own loopback port
	iface := rpciface.New(rpciface.Options{Backend: rpciface.ServiceBackend{Svc: svc}, Version: version, Log: logf,
		SubscribersFile: filepath.Join(varDir, "subscribers.json"),
		Names:           rpciface.MetaNames{Client: systemClient(o), Base: o.base}})
	svc.Subscribe(iface.Notify)
	svc.OnChange(iface.Changed)
	il, err := addonkit.Listen(o.rpcPort, "")
	if err != nil {
		return err
	}
	isrv := &http.Server{Handler: iface.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go iface.Run(ctx)
	go func() {
		if err := isrv.Serve(il); err != nil && err != http.ErrServerClosed {
			logf("the interface stopped: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = isrv.Shutdown(sctx)
	}()
	logf("the interface %s answers on %s", rpciface.Interface, il.Addr())

	wrap := func(h http.Handler) http.Handler { return h }
	if !o.noSession {
		c := addonkit.NewChecker(addonkit.SessionOptions{Base: o.base, Need: addonkit.LevelAdmin, AddonID: o.id, OnRefused: func(r *http.Request, reason string) {
			logf("refused %s %s: %s", r.Method, r.URL.Path, reason)
		}})
		wrap = c.Handler
	}
	h := httpapi.New(httpapi.Options{Service: svc, System: page{client: lite, http: systemClient(o), base: o.base}, BasePath: o.basePath, WWW: o.www, Wrap: wrap, Version: version, Started: time.Now(), DevClock: devClock})
	l, err := addonkit.Listen(o.port, "")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	logf("%s: serving %s on %s", version, o.basePath, l.Addr())
	if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
		return err
	}
	logf("stopping")
	writer.Close()
	sender.Close()
	<-saved
	if err := svc.Close(); err != nil {
		logf("the state could not be written: %v", err)
	}
	os.Stdout.Sync()
	return nil
}

// tokenSource is the addon's own token, or the one of -token-file.
func tokenSource(o options) addonkit.TokenSource {
	if o.tokenFile != "" {
		return &addonkit.FileToken{Path: o.tokenFile}
	}
	return addonkit.AddonToken(o.id)
}

// systemClient speaks to the system's API with that token.
func systemClient(o options) *http.Client {
	return &http.Client{Transport: &addonkit.AuthTransport{Source: tokenSource(o)}, Timeout: 30 * time.Second}
}

// page answers the page's questions to the system: which devices exist, what they are called,
// and the ReGa database kept after the switch from a CCU.
type page struct {
	client *litesource.Client
	http   *http.Client
	base   string
}

// Channels lists every channel of every interface but the addon's own.
func (s page) Channels(ctx context.Context) (map[string]bool, error) {
	ifs, err := s.client.Interfaces(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, i := range ifs {
		if i.Name == rpciface.Interface {
			continue
		}
		devs, err := s.client.Devices(ctx, i.Name)
		if err != nil {
			continue // an interface that is down: its devices count as missing
		}
		for _, d := range devs {
			out[i.Name+"/"+d.Address] = true
		}
	}
	return out, nil
}

// Names answers the names the system has for the channels.
func (s page) Names(ctx context.Context, channels []string) (map[string]string, error) {
	names, err := s.client.Names(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, ch := range channels {
		if iface, addr, ok := strings.Cut(ch, "/"); ok {
			if n := names.Name(iface, addr); n != "" {
				out[ch] = n
			}
		}
	}
	return out, nil
}

// Devices and Params answer the editor's picker from the system's device lists, metadata and
// paramset descriptions.
func (s page) Devices(ctx context.Context, lang string) ([]system.Device, error) {
	return (&system.Catalog{Lite: s.client, Own: rpciface.Interface}).Devices(ctx, lang)
}

func (s page) Params(ctx context.Context, channels []string) (map[string][]system.Param, error) {
	return (&system.Catalog{Lite: s.client, Own: rpciface.Interface}).Params(ctx, channels)
}

// Regadom reads the kept database.
func (s page) Regadom(ctx context.Context) (*regadom.Source, error) {
	return importer.FetchSystem(ctx, s.http, s.base)
}

// HasRegadom says whether the system keeps one: the route answers, and the body is not read.
func (s page) HasRegadom(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(s.base, "/")+importer.SystemPath, nil)
	if err != nil {
		return false
	}
	res, err := s.http.Do(req)
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}
