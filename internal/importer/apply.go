package importer

import (
	"errors"
	"io/fs"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
	"github.com/openccu-lite/lite-addon-logic/internal/store"
)

// Where the import keeps its own files in the tree.
const (
	ImportsDir = "imports"
	SeenPath   = "devices-seen.yaml"
)

// LedgerPath is a source's ledger inside the tree.
func LedgerPath(source string) string { return ImportsDir + "/" + source + ".yaml" }

// LoadLedger reads a source's ledger; a source never imported has none (nil, no error).
func LoadLedger(t *store.Tree, source string) (*Ledger, error) {
	if !ValidSource(source) {
		return nil, errors.New("import: the source's name must be lower-case letters, digits and hyphens")
	}
	data, err := t.Read(LedgerPath(source))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return DecodeLedger(data)
}

// InStore is Options.Taken for a loaded tree: a key is taken when the tree has a file of it. The
// source's own files are found through its ledger before this is asked.
func InStore(set *model.Set) func(kind, key string) bool {
	return func(kind, key string) bool {
		if kind == "program" {
			return set.Programs[key] != nil
		}
		return set.Variables[key] != nil
	}
}

// Files are the import's own files for the tree, by path: the source's ledger, and the notes on
// missing devices merged with the ones the tree has (have: devices-seen.yaml as it is, nil when
// there is none; notes nobody can read are replaced). They go into the same history entry as the
// programs and variables (service.Import.Files).
func (r *Result) Files(have []byte) map[string][]byte {
	seen := r.Seen
	if old, err := DecodeSeen(have); err == nil && have != nil {
		seen = MergeSeen(old, r.Seen)
	}
	return map[string][]byte{LedgerPath(r.Ledger.Source): r.Ledger.Encode(), SeenPath: EncodeSeen(seen)}
}
