// regaharness records the reference side of the differential harness (internal/harness): it
// starts the reference system's ReGaHss in a container under a faked clock, one container per
// scenario, plays the scenario and writes rega.trace, homematic.regadom.gz and programs.yaml
// into the scenario's testdata folder. With -compare it also plays the engine side at once and
// says whether the two agree.
//
//	regaharness [-image logic-rega:0254] [-out internal/harness/testdata] [-j 6] [-compare] [-logs dir] [scenario ...]
//
// Without names every scenario without a recording is recorded; -all records every one again.
// The image is built from internal/harness/rega (see its README): it needs the reference's
// ReGaHss binary, which is not part of this repository.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openccu-lite/lite-addon-logic/internal/harness"
)

// container is a reference in docker.
type container struct {
	name, image string
	port        int
	clock       string // the folder mounted at /clock
	http        *http.Client
}

func docker(args ...string) (string, error) {
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (c *container) setClock(at time.Time, rate int) error {
	return os.WriteFile(filepath.Join(c.clock, "offset"), []byte(harness.ClockOffset(at, time.Now(), rate)), 0o644)
}

func (c *container) start(at time.Time, rate int) error {
	_, _ = docker("rm", "-f", c.name)
	if err := c.setClock(at, rate); err != nil {
		return err
	}
	if _, err := docker("run", "-d", "--name", c.name, "-v", c.clock+":/clock", "-p", fmt.Sprintf("127.0.0.1:%d:8183", c.port), c.image); err != nil {
		return err
	}
	return c.ready()
}

func (c *container) ready() error {
	var err error
	for i := 0; i < 100; i++ {
		var out string
		if out, err = c.Script(`Write("up");`); err == nil && strings.Contains(out, "up") {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("%s does not answer: %v", c.name, err)
}

func (c *container) Script(text string) (string, error) {
	res, err := c.http.Post(fmt.Sprintf("http://127.0.0.1:%d/rega.exe", c.port), "text/plain", strings.NewReader(text))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	// the answer is what the script wrote, then an XML block with the script's variables
	if i := bytes.LastIndex(body, []byte("<xml><exec>")); i >= 0 {
		body = body[:i]
	}
	return string(body), nil
}

func (c *container) Regadom() ([]byte, error) {
	out, err := exec.Command("docker", "exec", c.name, "cat", "/etc/config/homematic.regadom").Output()
	if err != nil {
		return nil, fmt.Errorf("reading the database of %s: %v", c.name, err)
	}
	return out, nil
}

func (c *container) Restart(at time.Time, rate int) error {
	if _, err := docker("stop", "-t", "10", c.name); err != nil {
		return err
	}
	if err := c.setClock(at, rate); err != nil {
		return err
	}
	if _, err := docker("start", c.name); err != nil {
		return err
	}
	return c.ready()
}

func (c *container) remove(logs string) {
	if logs != "" {
		// the reference's own log, for looking into what it did
		if out, err := exec.Command("docker", "logs", c.name).CombinedOutput(); err == nil {
			_ = os.WriteFile(filepath.Join(logs, c.name+".log"), out, 0o644)
		}
	}
	_, _ = docker("rm", "-f", c.name)
	_ = os.RemoveAll(c.clock)
}

// record plays one scenario and writes its files.
func record(sc *harness.Scenario, image, out, logs string, port int, compare bool) (string, error) {
	clock, err := os.MkdirTemp("", "logic-rega-clock-")
	if err != nil {
		return "", err
	}
	_ = os.Chmod(clock, 0o755)
	// a connection per script: ReGaHss now and then ends a connection that is used again without
	// an answer (and without running the script) - three in a thousand were lost that way
	c := &container{name: "logic-rega-" + sc.Name, image: image, port: port, clock: clock, http: &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}}
	defer c.remove(logs)
	rate := sc.SpeedRate()
	// the programs are created in the ten real seconds before the scenario's start; the first
	// program with a time module makes the reference wait for its timer thread (up to 30 s of
	// its clock per program) and check the program once more, which must be over by the start
	lead := time.Duration(10+len(sc.Programs)/4) * time.Second * time.Duration(rate)
	if sc.Timed() {
		lead += time.Duration(60+35*len(sc.Programs)) * time.Second
	}
	if err := c.start(sc.StartTime().Add(-lead), rate); err != nil {
		return "", err
	}
	r := &harness.Recorder{Target: c}
	rec, dom, err := r.Record(sc)
	if err != nil {
		return "", err
	}
	im, err := harness.Import(sc, dom)
	if err != nil {
		return "", fmt.Errorf("the import of the recorded database: %v", err)
	}
	dir := filepath.Join(out, sc.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for name, data := range map[string][]byte{"rega.trace": rec.Encode(), "homematic.regadom.gz": harness.Gzip(dom), "programs.yaml": im.YAML(sc)} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return "", err
		}
	}
	verdict := fmt.Sprintf("recorded, %d entries", len(rec.Trace))
	if compare {
		got, err := harness.RunEngine(sc, im, rec)
		switch {
		case err != nil:
			verdict += "; engine: " + err.Error()
		default:
			if err := harness.Verdict(sc, rec, got); errors.Is(err, harness.ErrOpen) {
				verdict += "; " + err.Error()
			} else if err != nil {
				verdict += "; DIFFERS: " + err.Error()
			} else {
				verdict += "; the engine agrees (" + string(sc.Mark) + ")"
			}
		}
	}
	return verdict, nil
}

func main() {
	image := flag.String("image", "logic-rega:0254", "the reference's docker image")
	out := flag.String("out", "internal/harness/testdata", "where the recordings go")
	jobs := flag.Int("j", 6, "scenarios recorded at the same time")
	port := flag.Int("port", 18200, "the first local port")
	all := flag.Bool("all", false, "record every scenario again")
	compare := flag.Bool("compare", false, "play the engine side too and compare")
	list := flag.Bool("list", false, "list the scenarios")
	logs := flag.String("logs", "", "a folder that gets each reference's own log")
	flag.Parse()
	var todo []*harness.Scenario
	for _, sc := range harness.Scenarios() {
		if *list {
			fmt.Printf("%-32s %-10s %-9s %s\n", sc.Name, sc.Quirk, sc.Mark, sc.Title)
			continue
		}
		named := false
		for _, a := range flag.Args() {
			named = named || a == sc.Name
		}
		_, err := os.Stat(filepath.Join(*out, sc.Name, "rega.trace"))
		if named || *all || (flag.NArg() == 0 && err != nil) {
			todo = append(todo, sc)
		}
	}
	if *list {
		return
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	failed := 0
	slots := make(chan int, *jobs)
	for i := 0; i < *jobs; i++ {
		slots <- *port + i
	}
	for _, sc := range todo {
		wg.Add(1)
		go func(sc *harness.Scenario) {
			defer wg.Done()
			p := <-slots
			defer func() { slots <- p }()
			verdict, err := record(sc, *image, *out, *logs, p, *compare)
			for try := 0; err != nil && try < 2; try++ {
				// once more: a reference that did not start in time, or a setup that took too long
				// on a busy machine
				fmt.Printf("again %s: %v\n", sc.Name, err)
				verdict, err = record(sc, *image, *out, *logs, p, *compare)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				fmt.Printf("FAIL %s: %v\n", sc.Name, err)
				return
			}
			fmt.Printf("ok   %s: %s\n", sc.Name, verdict)
		}(sc)
	}
	wg.Wait()
	if failed > 0 {
		os.Exit(1)
	}
}
