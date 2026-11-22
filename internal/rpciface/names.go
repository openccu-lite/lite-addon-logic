package rpciface

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MetaNames writes names through the system's route for an addon's own interface
// (POST /api/meta/v1/interfaces/<interface>/objects:bulk, scope meta:interface).
type MetaNames struct {
	// Client carries the addon's token.
	Client *http.Client
	// Base is the system's URL.
	Base string
}

// Push sets the names of the given references and removes the others.
func (m MetaNames) Push(ctx context.Context, set map[string]string, remove []string) error {
	body := struct {
		Set    map[string]map[string]string `json:"set"`
		Delete []string                     `json:"delete,omitempty"`
	}{Set: map[string]map[string]string{}, Delete: remove}
	for ref, name := range set {
		body.Set[ref] = map[string]string{"name": name}
	}
	data, _ := json.Marshal(body) // strings only: cannot fail
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(m.Base, "/")+"/api/meta/v1/interfaces/"+Interface+"/objects:bulk", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// 304: the system has these names already - nothing changed, which is what was asked for
	if res.StatusCode/100 != 2 && res.StatusCode != http.StatusNotModified {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("the system answered %s: %s", res.Status, strings.TrimSpace(string(msg)))
	}
	return nil
}

// Known reads the names the metadata holds for this interface's references
// (GET /api/meta/v1/objects, scope meta:read).
func (m MetaNames) Known(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.Base, "/")+"/api/meta/v1/objects", nil)
	if err != nil {
		return nil, err
	}
	res, err := m.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("the system answered %s: %s", res.Status, strings.TrimSpace(string(msg)))
	}
	var body struct {
		Objects map[string]struct {
			Name string `json:"name"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("the system's list of names is not readable: %w", err)
	}
	out := map[string]string{}
	for ref, o := range body.Objects {
		if strings.HasPrefix(ref, Interface+".") {
			out[ref] = o.Name
		}
	}
	return out, nil
}
