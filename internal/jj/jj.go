// Package jj names what a jj repository is working on the way the shell
// prompt does: the nearest bookmark and how far behind it the working copy is
// (`main~1`), or the change id when no bookmark is near, with the prompt's
// status markers beside it. A colocated
// repository's git HEAD is always detached, so git alone can only offer a
// hash that changes with every new change.
//
// The label comes from jj-starship, the prompt's own tool, which reads the
// repository directly in a few milliseconds. What it said is remembered until
// the repository records another operation, so a poll costs one stat per
// repository while nothing changes.
package jj

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Binary is the program the label is asked of.
const Binary = "jj-starship"

// runTimeout bounds one run. jj-starship answers in milliseconds; a run that
// takes a second is stuck, and the title is better without it.
const runTimeout = time.Second

// Runner asks jj-starship about the repository rooted at root and returns what
// it printed.
type Runner func(ctx context.Context, root string) (string, error)

// Reader labels jj repositories, remembering each label until the repository
// it came from records another operation.
type Reader struct {
	run   Runner
	mu    sync.Mutex
	cache map[string]entry
}

// Label is what a workspace is working on and the state it is in.
type Label struct {
	// Name is `bookmark`, `bookmark~N` or a change id.
	Name string
	// Status is jj-starship's markers, such as `∅` or `!⇡`, or "" for none.
	Status string
}

// entry is a label and the operation-heads stamp it was read at.
type entry struct {
	stamp time.Time
	label Label
}

// New builds a reader that runs jj-starship from PATH. With no jj-starship to
// run, jj repositories get no label, which is said once here rather than on
// every poll.
func New(log *slog.Logger) *Reader {
	path, err := exec.LookPath(Binary)
	if err != nil {
		log.Info("jj repositories get no label", "reason", Binary+" not found on PATH")

		return NewWithRunner(nil)
	}

	return NewWithRunner(func(ctx context.Context, root string) (string, error) {
		// No shell: root is one argv element, absolute so it never reads as a
		// flag.
		//nolint:forbidigo,gosec // a fixed binary and an absolute path, no shell
		out, err := exec.CommandContext(ctx, path, "--cwd", root,
			"--no-color", "--no-symbol", "--no-jj-prefix").Output()

		return string(out), err
	})
}

// NewWithRunner builds a reader around run. A nil run labels nothing.
func NewWithRunner(run Runner) *Reader {
	return &Reader{run: run, cache: make(map[string]entry)}
}

// Root walks up from dir to the directory holding a .jj directory, the root of
// the jj workspace dir is in.
func Root(dir string) (string, bool) {
	if dir == "" || !filepath.IsAbs(dir) {
		return "", false
	}

	for dir = filepath.Clean(dir); ; {
		if info, err := os.Stat(filepath.Join(dir, ".jj")); err == nil && info.IsDir() {
			return dir, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}

		dir = parent
	}
}

// Label reports what the workspace rooted at root is working on: `bookmark` or
// `bookmark~N` when a bookmark is on the working copy or near behind it, the
// change id otherwise, and the zero Label when jj-starship cannot say.
func (r *Reader) Label(ctx context.Context, root string) Label {
	if r.run == nil {
		return Label{}
	}

	// No stamp means no repository to read the operation log of; a label
	// would be read again on every poll, so there is none.
	stamp, ok := opStamp(root)
	if !ok {
		return Label{}
	}

	r.mu.Lock()
	cached, known := r.cache[root]
	r.mu.Unlock()

	if known && cached.stamp.Equal(stamp) {
		return cached.label
	}

	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	out, err := r.run(ctx, root)
	if err != nil {
		// Not remembered: a timeout or a failed run may not happen again.
		return Label{}
	}

	label := Parse(out)

	r.mu.Lock()
	r.cache[root] = entry{stamp: stamp, label: label}
	r.mu.Unlock()

	return label
}

// Parse reduces jj-starship's output, `<change id>[ (<bookmark>[~N][, …])][
// [<status>]]`, to the first bookmark when there is one and the change id
// otherwise, with the status markers apart.
func Parse(out string) Label {
	out = strings.TrimSpace(out)
	if out == "" || strings.ContainsAny(out, "\n\r") {
		return Label{}
	}

	var status string
	if open := strings.LastIndex(out, " ["); open >= 0 && strings.HasSuffix(out, "]") {
		out, status = out[:open], out[open+2:len(out)-1]
	}

	id, rest, hasBookmarks := strings.Cut(out, " (")
	if !hasBookmarks {
		if strings.ContainsAny(id, " ()[]") {
			return Label{}
		}

		return Label{Name: id, Status: status}
	}

	bookmarks, closed := strings.CutSuffix(rest, ")")
	if !closed {
		return Label{}
	}

	first, _, _ := strings.Cut(bookmarks, ",")
	if first = strings.TrimSpace(first); first == "" {
		return Label{Name: id, Status: status}
	}

	return Label{Name: first, Status: status}
}

// opStamp reads when the repository's operation heads last changed, which
// every jj operation does. A workspace other than the first keeps a file at
// .jj/repo naming the repository it shares.
func opStamp(root string) (time.Time, bool) {
	repo, err := repoDir(filepath.Join(root, ".jj"))
	if err != nil {
		return time.Time{}, false
	}

	info, err := os.Stat(filepath.Join(repo, "op_heads", "heads"))
	if err != nil {
		return time.Time{}, false
	}

	return info.ModTime(), true
}

// errNoRepo is a .jj/repo that is neither a directory nor names one.
var errNoRepo = errors.New("no jj repository")

// repoDir resolves .jj/repo to the repository directory, following the
// pointer file a secondary workspace keeps there. jj writes that path
// relative to the .jj directory or absolute.
func repoDir(jjDir string) (string, error) {
	repo := filepath.Join(jjDir, "repo")

	info, err := os.Stat(repo)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		return repo, nil
	}

	target, err := os.ReadFile(repo) //nolint:gosec // located by this package
	if err != nil {
		return "", err
	}

	path := strings.TrimSpace(string(target))
	if path == "" {
		return "", errNoRepo
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(jjDir, path)
	}

	return filepath.Clean(path), nil
}
