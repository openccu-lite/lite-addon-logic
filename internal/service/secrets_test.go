package service

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/engine"
	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

type calls struct{ list []engine.Call }

func (c *calls) Call(call engine.Call, done func(engine.CallResult)) bool {
	c.list = append(c.list, call)
	return true
}

func hooks(name string, texts ...string) *model.Program {
	p := &model.Program{Name: name, Visible: true, Operable: true, Rules: []model.Rule{{When: &model.Condition{Op: model.All}}}}
	for _, t := range texts {
		p.Rules[0].Then = append(p.Rules[0].Then, model.Action{Kind: model.ActionWebhook, Webhook: &model.Webhook{Method: "GET", URL: "http://h/?k=" + t}})
	}
	p.Rules[0].Then = append(p.Rules[0].Then, model.Action{Kind: model.ActionWake, Wake: &model.Wake{MAC: "AA:BB:CC:00:11:22"}})
	return p
}

// Secrets are entered, replaced and removed; the file is the addon's alone and outside the
// tree; a secret programs name cannot be removed, and one they name that is not there is said.
func TestSecrets(t *testing.T) {
	dir := t.TempDir()
	s, _ := open(t, dir)
	if list, missing := s.Secrets(); len(list) != 0 || len(missing) != 0 {
		t.Fatalf("at first: %v %v", list, missing)
	}
	for _, c := range []struct{ name, value, code string }{
		{"token", "abc", ""},
		{"token", "s3cret", ""},
		{"other-1", "x", ""},
		{"Token", "x", "invalid"},
		{"", "x", "invalid"},
		{"empty", "", "invalid"},
		{"long", strings.Repeat("x", MaxSecret+1), "invalid"},
	} {
		if got := codeOf(s.SetSecret(c.name, c.value)); got != c.code {
			t.Errorf("set %q: %s", c.name, got)
		}
	}
	if v, ok := s.Secret("token"); v != "s3cret" || !ok {
		t.Errorf("token: %q", v)
	}
	if _, ok := s.Secret("nope"); ok {
		t.Error("a secret nobody entered")
	}
	path := filepath.Join(dir, "secrets.json")
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the file: %v %v", fi, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tree", "secrets.json")); err == nil {
		t.Error("the secrets are in the tree")
	}

	if _, err := s.CreateProgram(hooks("Melden", "{secret:token}", "{secret:missing}{secret:token}", "{value}"), admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProgram(hooks("Zweites", "{secret:missing}"), admin); err != nil {
		t.Fatal(err)
	}
	list, missing := s.Secrets()
	if !reflect.DeepEqual(list, []Secret{{"other-1", []string{}}, {"token", []string{"melden"}}}) || !reflect.DeepEqual(missing, []string{"missing"}) {
		t.Errorf("secrets %v, missing %v", list, missing)
	}
	// a command names secrets as a webhook does
	third := &model.Program{Name: "Drittes", Visible: true, Operable: true, Rules: []model.Rule{{When: &model.Condition{Op: model.All}, Then: []model.Action{
		{Kind: model.ActionCommand, Command: &model.Command{Args: []model.CommandArg{{Text: "true"}, {Text: "{secret:other-1}{secret:fehlt}"}}}}}}}}
	if _, err := s.CreateProgram(third, admin); err != nil {
		t.Fatal(err)
	}
	if list, missing := s.Secrets(); !reflect.DeepEqual(list[0], Secret{"other-1", []string{"drittes"}}) || !reflect.DeepEqual(missing, []string{"fehlt", "missing"}) {
		t.Errorf("with a command: secrets %v, missing %v", list, missing)
	}
	if err := s.DeleteProgram("drittes", admin); err != nil {
		t.Fatal(err)
	}
	if got := codeOf(s.DeleteSecret("token")); got != "in-use" {
		t.Errorf("in use: %s", got)
	}
	if got := codeOf(s.DeleteSecret("nope")); got != "not-found" {
		t.Errorf("not there: %s", got)
	}
	if got := codeOf(s.DeleteSecret("other-1")); got != "" {
		t.Errorf("removed: %s", got)
	}

	// the file cannot be written: nothing changes
	os.Remove(path)
	os.Mkdir(path, 0o700)
	os.WriteFile(filepath.Join(path, "x"), nil, 0o600)
	if err := s.SetSecret("token", "new"); err == nil || codeOf(err) != "other: "+err.Error() {
		t.Errorf("replaced without a file: %v", err)
	}
	if err := s.SetSecret("fresh", "new"); err == nil {
		t.Error("entered without a file")
	}
	if err := s.DeleteSecret("fresh"); codeOf(err) != "not-found" {
		t.Errorf("fresh: %v", err)
	}
	s.mu.Lock()
	delete(s.set.Programs, "melden")
	s.mu.Unlock()
	if err := s.DeleteSecret("token"); err == nil || codeOf(err) == "in-use" {
		t.Errorf("removed without a file: %v", err)
	}
	if v, _ := s.Secret("token"); v != "s3cret" {
		t.Errorf("token after the failures: %q", v)
	}
	s.Close()

	// a file that cannot be read, and a damaged one: said, and the addon starts
	s, logs := open(t, dir)
	if _, ok := s.Secret("token"); ok || !strings.Contains(strings.Join(*logs, "\n"), "could not be read") {
		t.Errorf("a directory as the file: %v", *logs)
	}
	s.Close()
	os.RemoveAll(path)
	os.WriteFile(path, []byte("null"), 0o600)
	s, logs = open(t, dir)
	if !strings.Contains(strings.Join(*logs, "\n"), "damaged") {
		t.Errorf("a damaged file: %v", *logs)
	}
	// at most MaxSecrets
	for i := 0; i < MaxSecrets; i++ {
		if err := s.SetSecret(fmt.Sprintf("s-%d", i), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if got := codeOf(s.SetSecret("one-more", "x")); got != "invalid" {
		t.Errorf("one more: %s", got)
	}
	s.Close()
	s, _ = open(t, dir)
	if v, ok := s.Secret("s-7"); !ok || v != "x" {
		t.Error("the secrets after a restart")
	}
	s.Close()
}

// The service hands its caller to the engine.
func TestCallerIsTheEngines(t *testing.T) {
	c := &calls{}
	s, err := Open(Options{Dir: t.TempDir(), Caller: c})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p := hooks("Melden", "x")
	p.Active = true
	if _, err := s.CreateProgram(p, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.Engine().RunProgramBy("melden", model.RunThen, "user"); err != nil {
		t.Fatal(err)
	}
	if len(c.list) != 2 || c.list[0].Action.Webhook == nil || c.list[1].Action.Wake == nil {
		t.Errorf("calls %+v", c.list)
	}
}
