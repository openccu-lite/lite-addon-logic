package importer

import (
	"regexp"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// A script that is one system.Exec (regadom.Exec) becomes an activity of this addon: a call
// of wget or curl that only fetches an address a webhook, ether-wake a Wake-on-LAN, and
// everything else a command with the line as the CCU's shell got it. A command's program is
// imported inactive: on the CCU the line ran as root, here it runs with the addon's rights.

var placeholderLike = regexp.MustCompile(`\{(program|trigger|value|time|var:[a-z0-9-]+|secret:[a-z0-9-]+)\}`)

// webhook makes a fetch a webhook; false when a webhook cannot say it.
func webhook(f *regadom.Fetch) (*model.Webhook, bool) {
	w := &model.Webhook{Method: f.Method, URL: f.URL, Body: f.Body, Insecure: f.Insecure}
	texts := []string{w.URL, w.Body}
	for _, h := range f.Headers {
		w.Headers = append(w.Headers, model.Header{Name: h[0], Value: h[1]})
		texts = append(texts, h[1])
	}
	if w.Body != "" {
		// the body goes out as it is written: with the type the line names, else as a form
		switch {
		case f.JSON:
			w.BodyType = model.BodyJSON
		case f.Typed:
			w.BodyType = model.BodyText
		default:
			w.BodyType = model.BodyForm
		}
	}
	for _, t := range texts {
		// what would be a placeholder here was plain text on the CCU
		if placeholderLike.MatchString(t) {
			return nil, false
		}
	}
	return w, w.Check() == ""
}

// exec maps a recognised system.Exec; false when it stays a script.
func (m *imp) exec(call regadom.ExecCall, st *prog) (model.Action, bool) {
	var out model.Action
	if !call.Output {
		if f, ok := call.Fetch(); ok {
			if hook, ok := webhook(f); ok {
				out.Kind, out.Webhook = model.ActionWebhook, hook
				st.note(NoteExecWebhook)
				m.hist["activity: system.Exec (webhook)"]++
				return out, true
			}
		}
		if mac, broadcast, ok := call.Wake(); ok {
			if wake := (&model.Wake{MAC: model.CanonicalMAC(mac), Broadcast: broadcast}); wake.Check() == "" {
				out.Kind, out.Wake = model.ActionWake, wake
				st.note(NoteExecWake)
				m.hist["activity: system.Exec (Wake-on-LAN)"]++
				return out, true
			}
		}
	}
	c := &model.Command{Shell: true, Line: call.Command, Wait: true}
	if call.Output {
		def := m.vars[call.OutID]
		if call.OutName != "" {
			def = m.variableNamed(call.OutName)
		}
		if def == nil || def.Computed != nil || (def.Type != model.VarString && def.Type != model.VarNumber && def.Type != model.VarBool) {
			return out, false
		}
		c.Output = def.Key
	}
	if c.Check() != "" {
		return out, false
	}
	out.Kind, out.Command = model.ActionCommand, c
	st.reason(ReasonCommand)
	if call.NeedsRoot() {
		st.note(NoteCommandRoot)
	}
	m.hist["activity: system.Exec (command)"]++
	return out, true
}

// variableNamed answers the imported variable with a name on the CCU (the one with the lowest
// id when several have it).
func (m *imp) variableNamed(name string) *model.Variable {
	best := -1
	for id, v := range m.vars {
		if v.Name == name && (best < 0 || id < best) {
			best = id
		}
	}
	return m.vars[best]
}
