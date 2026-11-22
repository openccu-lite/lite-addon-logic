package system

import (
	"context"
	"sync"
	"time"

	"github.com/hobbyquaker/ccu-addon-kit/go/litesource"

	"github.com/openccu-lite/lite-addon-logic/internal/model"
)

// NameSource is the slice of the kit's client the names use.
type NameSource interface {
	Names(ctx context.Context) (*litesource.Names, error)
}

// Names answers the devices' names for the comments a program's file gets at save time. The
// metadata is read when a name is asked for and what was read is older than a minute; when the
// system does not answer the comment is left out - a save never waits long for a name.
type Names struct {
	Lite NameSource
	// Now is the clock (time.Now when nil).
	Now func() time.Time

	mu    sync.Mutex
	names *litesource.Names
	read  time.Time
}

// Name answers the name of a datapoint's channel, or of its device, or "".
func (n *Names) Name(ref model.Ref) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now
	if n.Now != nil {
		now = n.Now
	}
	if n.read.IsZero() || now().Sub(n.read) > time.Minute {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		names, err := n.Lite.Names(ctx)
		cancel()
		n.read = now()
		if err == nil {
			n.names = names
		}
	}
	if n.names == nil {
		return ""
	}
	if name := n.names.Name(ref.Interface, ref.Address); name != "" {
		return name
	}
	for i := len(ref.Address) - 1; i >= 0; i-- {
		if ref.Address[i] == ':' {
			return n.names.Name(ref.Interface, ref.Address[:i])
		}
	}
	return ""
}
