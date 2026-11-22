package importer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openccu-lite/lite-addon-logic/internal/regadom"
)

// SystemPath is the system's route to the ReGa database kept after the switch from a CCU
// (scope regadom:read).
const SystemPath = "/api/system/v1/regadom/file"

// ErrNoSystemDatabase says the system keeps no ReGa database: it never was a CCU, or the user
// removed the file.
var ErrNoSystemDatabase = errors.New("the system keeps no ReGa database of a CCU")

// FetchSystem reads the database the system kept. client carries the addon's token; base is the
// system's URL.
func FetchSystem(ctx context.Context, client *http.Client, base string) (*regadom.Source, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+SystemPath, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the system's ReGa database: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNoSystemDatabase
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("the system's ReGa database: the system answered %s", res.Status)
	}
	return regadom.Read(res.Body)
}
