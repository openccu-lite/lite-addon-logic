package httpapi

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
)

func TestRunsAPI(t *testing.T) {
	e := newEnv(t)
	who := repo.Author{Name: "Test"}
	if _, err := e.svc.CreateVariable(&model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"markise", "andere"} {
		prg := &model.Program{Key: key, Name: key, Active: true, Visible: true, Operable: true, Rules: []model.Rule{
			{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "regen", Op: model.OpEq, Value: true, Trigger: model.TriggerChange}}}}},
				Then: []model.Action{{Kind: model.ActionVariable, Variable: "regen", To: true, After: model.Duration(3600e9)}}}}}
		if _, err := e.svc.CreateProgram(prg, who); err != nil {
			t.Fatal(err)
		}
	}
	out := e.want(http.StatusOK, "", "GET", "runs", nil)
	if runs := out["runs"].([]any); len(runs) != 0 || out["more"] != false {
		t.Fatalf("before anything ran: %v", out)
	}
	eng := e.svc.Engine()
	for _, v := range []bool{true, true, false, true} {
		if err := eng.SetVariable("regen", v, 0); err != nil {
			t.Fatal(err)
		}
	}
	// the global log, the newest first: every run with its trigger, its lines and its steps
	out = e.want(http.StatusOK, "", "GET", "runs", nil)
	runs := out["runs"].([]any)
	if len(runs) != 6 || out["more"] != false {
		t.Fatalf("%d entries: %v", len(runs), out)
	}
	first := runs[0].(map[string]any)
	line := first["lines"].([]any)[0].(map[string]any)
	step := first["steps"].([]any)[0].(map[string]any)
	if first["trigger"] != "variable:regen" || first["block"] != 1.0 || first["event"].(map[string]any)["prev"] != false ||
		line["truth"] != true || line["value"] != true || line["def"].(map[string]any)["variable"] != "regen" ||
		step["what"] != "scheduled" || step["due"] == nil || step["def"].(map[string]any)["variable"] != "regen" {
		t.Errorf("the newest entry: %v", first)
	}
	// a page, and the one below it
	out = e.want(http.StatusOK, "", "GET", "runs?limit=4", nil)
	page := out["runs"].([]any)
	if len(page) != 4 || out["more"] != true {
		t.Fatalf("a page of four: %v", out)
	}
	seq := page[3].(map[string]any)["seq"].(float64)
	out = e.want(http.StatusOK, "", "GET", "runs?limit=4&before="+fmt.Sprint(uint64(seq)), nil)
	if rest := out["runs"].([]any); len(rest) != 2 || out["more"] != false || rest[0].(map[string]any)["seq"].(float64) >= seq {
		t.Errorf("the page below: %v", out)
	}
	// one program's entries hold the note that it was not triggered, too
	out = e.want(http.StatusOK, "", "GET", "runs?program=markise", nil)
	mine := out["runs"].([]any)
	if len(mine) != 4 || mine[2].(map[string]any)["info"] != "no-change" || mine[2].(map[string]any)["untriggered"] != true {
		t.Errorf("markise: %v", out)
	}
	if out = e.want(http.StatusOK, "", "GET", "runs?program=nichts", nil); len(out["runs"].([]any)) != 0 {
		t.Errorf("an unknown program: %v", out)
	}
	for _, q := range []string{"limit=0", "limit=501", "limit=x", "before=-1", "before=x"} {
		e.want(http.StatusBadRequest, "invalid", "GET", "runs?"+q, nil)
	}
}

func TestDryRunAPI(t *testing.T) {
	e := newEnv(t)
	who := repo.Author{Name: "Test"}
	if _, err := e.svc.CreateVariable(&model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	prg := &model.Program{Key: "markise", Name: "Markise", Active: true, Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "regen", Op: model.OpEq, Value: true, Trigger: model.TriggerChange}}}}},
			Then: []model.Action{{Kind: model.ActionVariable, Variable: "regen", To: false}}}}}
	if _, err := e.svc.CreateProgram(prg, who); err != nil {
		t.Fatal(err)
	}
	// now: the condition is false and there is no SONST
	out := e.want(http.StatusOK, "", "POST", "programs/markise/dry-run", map[string]any{})
	if out["dry"] != true || out["none"] != true || out["trigger"] != "user" || len(out["lines"].([]any)) != 1 {
		t.Errorf("the dry run: %v", out)
	}
	// if it rained: the block would run and write - and nothing is written
	out = e.want(http.StatusOK, "", "POST", "programs/markise/dry-run", map[string]any{"line": 1, "value": true})
	if out["block"] != 1.0 || out["steps"].([]any)[0].(map[string]any)["what"] != "written" || out["event"].(map[string]any)["value"] != true {
		t.Errorf("the simulation: %v", out)
	}
	if v, _ := e.svc.Engine().Value("regen"); v != false {
		t.Errorf("the simulation wrote: %v", v)
	}
	if got := e.want(http.StatusOK, "", "GET", "runs", nil)["runs"].([]any); len(got) != 0 {
		t.Errorf("the simulation left entries: %v", got)
	}
	out = e.want(http.StatusOK, "", "POST", "programs/markise/dry-run", map[string]any{"line": 1, "value": false})
	if out["untriggered"] != true || out["info"] != "no-change" {
		t.Errorf("not triggered: %v", out)
	}
	e.want(http.StatusUnprocessableEntity, "wrong-type", "POST", "programs/markise/dry-run", map[string]any{"line": 1, "value": "nass"})
	e.want(http.StatusUnprocessableEntity, "invalid", "POST", "programs/markise/dry-run", map[string]any{"line": 2, "value": true})
	e.want(http.StatusNotFound, "unknown-program", "POST", "programs/nichts/dry-run", map[string]any{})
	e.want(http.StatusBadRequest, "invalid-body", "POST", "programs/markise/dry-run", map[string]any{"zeile": 1})
}

// The overview names every channel a program uses - also one an activity only reads its value
// from.
func TestOverviewNamesTheChannelAValueIsTakenFrom(t *testing.T) {
	e := newEnv(t)
	who := repo.Author{Name: "Test"}
	if _, err := e.svc.CreateVariable(&model.Variable{Key: "ziel", Name: "Ziel", Type: model.VarNumber, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	from := model.Ref{Interface: "HmIP-RF", Address: "0002:1", Param: "ACTUAL_TEMPERATURE"}
	prg := &model.Program{Key: "kopie", Name: "Kopie", Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.All, Groups: []model.Group{}}, Then: []model.Action{{Kind: model.ActionVariable, Variable: "ziel", FromDevice: &from}}}}}
	if _, err := e.svc.CreateProgram(prg, who); err != nil {
		t.Fatal(err)
	}
	e.sys.names = map[string]string{"HmIP-RF/0002:1": "Thermostat Bad"}
	out := e.want(http.StatusOK, "", "GET", "overview", nil)
	if names := out["names"].(map[string]any); names["HmIP-RF/0002:1"] != "Thermostat Bad" {
		t.Errorf("names: %v", names)
	}
}
