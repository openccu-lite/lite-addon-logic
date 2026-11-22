package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// The secrets a webhook's {secret:<name>} stands for: entered once in the settings, kept in a
// file only the addon reads - beside the tree, so neither in the files' history nor in an
// export - and never shown or logged again.

// Limits of the secrets.
const (
	MaxSecrets = 50
	MaxSecret  = 4096
)

// Secret is a secret as the page sees it: its name and who uses it, never its value.
type Secret struct {
	Name string `json:"name"`
	// Users are the programs with a webhook that names it.
	Users []string `json:"users"`
}

func (s *Service) secretsPath() string { return filepath.Join(s.o.Dir, "secrets.json") }

// loadSecrets reads the file; one that cannot be read is said and left alone.
func (s *Service) loadSecrets() {
	s.secrets = map[string]string{}
	data, err := os.ReadFile(s.secretsPath())
	if err != nil {
		if !os.IsNotExist(err) {
			s.logf("the secrets could not be read: %v", err)
		}
		return
	}
	if err := json.Unmarshal(data, &s.secrets); err != nil || s.secrets == nil {
		s.secrets = map[string]string{}
		s.logf("the secrets file is damaged: webhooks that need a secret fail until it is entered again")
	}
}

func (s *Service) saveSecrets() error {
	data, _ := json.MarshalIndent(s.secrets, "", "  ")
	return store.WriteFile(s.secretsPath(), append(data, '\n'), 0o600)
}

// Secret answers a secret's value: the webhooks' worker asks.
func (s *Service) Secret(name string) (string, bool) {
	s.secMu.Lock()
	defer s.secMu.Unlock()
	v, ok := s.secrets[name]
	return v, ok
}

// placeholders lists the placeholders of an activity that can name a secret: a webhook's and a
// command's.
func placeholders(a *model.Action) []string {
	switch {
	case a.Webhook != nil:
		return a.Webhook.Placeholders()
	case a.Command != nil:
		return a.Command.Placeholders()
	}
	return nil
}

// secretUsers lists the programs that name a secret, sorted (with the service locked).
func (s *Service) secretUsers(name string) []string {
	out := []string{}
	for k, p := range s.set.Programs {
		used := false
		p.Actions(func(_ int, a *model.Action) {
			for _, ph := range placeholders(a) {
				used = used || ph == "secret:"+name
			}
		})
		if used {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Secrets lists the secrets' names with their users, sorted - and, with a name only, the
// secrets programs name that are not entered (Missing).
func (s *Service) Secrets() (list []Secret, missing []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secMu.Lock()
	defer s.secMu.Unlock()
	list = []Secret{}
	for name := range s.secrets {
		list = append(list, Secret{Name: name, Users: s.secretUsers(name)})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	seen := map[string]bool{}
	missing = []string{}
	for _, p := range s.set.Programs {
		p.Actions(func(_ int, a *model.Action) {
			for _, ph := range placeholders(a) {
				if name, ok := strings.CutPrefix(ph, "secret:"); ok && !seen[name] {
					seen[name] = true
					if _, there := s.secrets[name]; !there {
						missing = append(missing, name)
					}
				}
			}
		})
	}
	sort.Strings(missing)
	return list, missing
}

// SetSecret enters a secret or replaces its value.
func (s *Service) SetSecret(name, value string) error {
	if !model.ValidKey(name) {
		return refuse("invalid", "a secret's name is made of lower-case letters, digits and hyphens (at most 48)")
	}
	if value == "" || len(value) > MaxSecret {
		return refuse("invalid", "a secret is 1 to %d bytes long", MaxSecret)
	}
	s.secMu.Lock()
	defer s.secMu.Unlock()
	old, had := s.secrets[name]
	if !had && len(s.secrets) >= MaxSecrets {
		return refuse("invalid", "there are %d secrets already", MaxSecrets)
	}
	s.secrets[name] = value
	if err := s.saveSecrets(); err != nil {
		if had {
			s.secrets[name] = old
		} else {
			delete(s.secrets, name)
		}
		return fmt.Errorf("the secrets could not be written: %w", err)
	}
	return nil
}

// DeleteSecret removes a secret; one that programs name cannot be removed.
func (s *Service) DeleteSecret(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secMu.Lock()
	defer s.secMu.Unlock()
	old, had := s.secrets[name]
	if !had {
		return refuse("not-found", "there is no secret %s", name)
	}
	if users := s.secretUsers(name); len(users) > 0 {
		return &Error{Code: "in-use", Msg: fmt.Sprintf("the secret %s cannot be removed while programs use it", name), Users: users}
	}
	delete(s.secrets, name)
	if err := s.saveSecrets(); err != nil {
		s.secrets[name] = old
		return fmt.Errorf("the secrets could not be written: %w", err)
	}
	return nil
}
