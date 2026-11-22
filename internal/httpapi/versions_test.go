package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/repo"
)

func TestVersionsAPI(t *testing.T) {
	e := newEnv(t)
	who := repo.Author{Name: "Test"}
	if _, err := e.svc.CreateVariable(&model.Variable{Key: "regen", Name: "Regen", Type: model.VarBool, Visible: true}, nil, who); err != nil {
		t.Fatal(err)
	}
	// nothing but its creation yet
	out := e.want(http.StatusOK, "", "GET", "variables/regen/versions", nil)
	if vs := out["versions"].([]any); len(vs) != 1 || !strings.Contains(out["current"].(string), "name: Regen") {
		t.Fatalf("the variable's versions: %v", out)
	}
	prg := &model.Program{Key: "markise", Name: "Markise", Visible: true, Operable: true, Rules: []model.Rule{
		{When: &model.Condition{Op: model.Any, Groups: []model.Group{{Op: model.All, Lines: []model.Line{{Variable: "regen", Op: model.OpEq, Value: true, Trigger: model.TriggerChange}}}}},
			Then: []model.Action{{Kind: model.ActionVariable, Variable: "regen", To: false}}}}}
	if _, err := e.svc.CreateProgram(prg, who); err != nil {
		t.Fatal(err)
	}
	// (the history is written in the background, with the files as they are when its turn
	// comes: two saves in a row may be one entry - here each gets its own)
	e.svc.Flush()
	e.want(http.StatusOK, "", "POST", "programs/markise/active", map[string]any{"active": true})
	out = e.want(http.StatusOK, "", "GET", "programs/markise/versions", nil)
	vs := out["versions"].([]any)
	if len(vs) != 2 || !strings.Contains(out["current"].(string), "active: true") {
		t.Fatalf("the program's versions: %v", out)
	}
	newest, first := vs[0].(map[string]any), vs[1].(map[string]any)
	if newest["message"] != "Program Markise changed" || first["message"] != "Program Markise created" || first["author"] != "Test" || first["time"] == nil {
		t.Errorf("entries: %v", vs)
	}
	hash := first["hash"].(string)
	text := e.want(http.StatusOK, "", "GET", "programs/markise/versions/"+hash, nil)["text"].(string)
	if !strings.Contains(text, "name: Markise") || strings.Contains(text, "active: true") {
		t.Errorf("the first version: %s", text)
	}
	// restore it: the program is off again, and the history says what happened
	e.want(http.StatusOK, "", "POST", "programs/markise/versions/"+hash+"/restore", nil)
	if p, _ := e.svc.Program("markise"); p.Program.Active {
		t.Error("the restored program is still active")
	}
	out = e.want(http.StatusOK, "", "GET", "programs/markise/versions", nil)
	if vs := out["versions"].([]any); len(vs) != 3 || vs[0].(map[string]any)["message"] != "Program Markise restored to its version "+hash[:7] || out["current"] != text {
		t.Errorf("after the restore: %v", out)
	}
	e.want(http.StatusNotFound, "not-found", "GET", "programs/nichts/versions", nil)
	e.want(http.StatusNotFound, "not-found", "GET", "programs/markise/versions/abc", nil)
	e.want(http.StatusNotFound, "not-found", "POST", "programs/markise/versions/abc/restore", nil)
	e.want(http.StatusNotFound, "not-found", "GET", "variables/nichts/versions", nil)
}
