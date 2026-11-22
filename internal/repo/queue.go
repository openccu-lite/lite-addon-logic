package repo

import (
	"sort"
	"sync"
)

// Queue commits in the background, so a save never waits for the history and never fails with
// it: a save hands its paths over and returns. A commit that fails keeps its paths; the next
// one, whoever asks for it, records them too.
type Queue struct {
	repo *Repo
	// OnError is told about a commit that failed (nil: nobody); OnCommit about one that was made.
	OnError  func(error)
	OnCommit func(hash, message string)

	mu      sync.Mutex
	pending []job
	failed  map[string]bool
	wake    chan struct{}
	done    chan struct{}
	idle    *sync.Cond
	busy    bool
	lastErr error
}

type job struct {
	paths   []string
	author  Author
	message string
}

// NewQueue starts the queue's worker; Close ends it after what is queued.
func NewQueue(r *Repo) *Queue {
	q := &Queue{repo: r, failed: map[string]bool{}, wake: make(chan struct{}, 1), done: make(chan struct{})}
	q.idle = sync.NewCond(&q.mu)
	go q.run()
	return q
}

// Repo is the repository the queue commits to.
func (q *Queue) Repo() *Repo { return q.repo }

// Enqueue asks for a commit of the paths.
func (q *Queue) Enqueue(paths []string, author Author, message string) {
	q.mu.Lock()
	q.pending = append(q.pending, job{paths: append([]string(nil), paths...), author: author, message: message})
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// Flush waits until everything queued so far has been tried.
func (q *Queue) Flush() {
	q.mu.Lock()
	for len(q.pending) > 0 || q.busy {
		q.idle.Wait()
	}
	q.mu.Unlock()
}

// Err answers the last commit's error, nil when it worked.
func (q *Queue) Err() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.lastErr
}

// Close ends the worker after what is queued.
func (q *Queue) Close() {
	q.Flush()
	close(q.wake)
	<-q.done
}

func (q *Queue) run() {
	defer close(q.done)
	for range q.wake {
		for {
			q.mu.Lock()
			if len(q.pending) == 0 {
				q.mu.Unlock()
				break
			}
			j := q.pending[0]
			q.pending = q.pending[1:]
			q.busy = true
			// what an earlier failed commit left unrecorded goes with this one
			set := map[string]bool{}
			for _, p := range j.paths {
				set[p] = true
			}
			for p := range q.failed {
				set[p] = true
			}
			q.mu.Unlock()
			paths := make([]string, 0, len(set))
			for p := range set {
				paths = append(paths, p)
			}
			sort.Strings(paths)
			hash, err := q.repo.Commit(paths, j.author, j.message)
			q.mu.Lock()
			q.lastErr = err
			if err != nil {
				for _, p := range paths {
					q.failed[p] = true
				}
			} else {
				q.failed = map[string]bool{}
			}
			onErr, onCommit := q.OnError, q.OnCommit
			q.mu.Unlock()
			// the listeners first, then "idle": who waits in Flush finds them told
			if err != nil && onErr != nil {
				onErr(err)
			}
			if err == nil && hash != "" && onCommit != nil {
				onCommit(hash, j.message)
			}
			q.mu.Lock()
			q.busy = false
			q.idle.Broadcast()
			q.mu.Unlock()
		}
	}
}
