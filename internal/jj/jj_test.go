package jj

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		out  string
		want string
	}{
		{"a bookmark on the working copy", "lsmuummm (demo)\n", "demo"},
		{"a bookmark behind the working copy keeps its distance", "ymznnlvz (demo~2)\n", "demo~2"},
		{
			"the nearest of several bookmarks is named",
			"ymznnlvz (other~1, third~1, demo~2)\n",
			"other~1",
		},
		{"no bookmark names the change", "xkurvtuq\n", "xkurvtuq"},
		{"empty parentheses fall back to the change", "xkurvtuq ()", "xkurvtuq"},
		{"nothing printed names nothing", "", ""},
		{"an unclosed list is not trusted", "xkurvtuq (main~1", ""},
		{"more than one line is not a label", "xkurvtuq\nerror: something", ""},
		{"a sentence is not a change id", "There is no jj repo", ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := Parse(test.out); got != test.want {
				t.Errorf("Parse(%q) = %q, want %q", test.out, got, test.want)
			}
		})
	}
}

// workspace lays down the files a jj workspace is recognised by, rather than
// running jj, and returns its root.
func workspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(
		filepath.Join(root, ".jj", "repo", "op_heads", "heads"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	return root
}

// touch moves the operation-heads stamp, as a jj operation would.
func touch(t *testing.T, root string, at time.Time) {
	t.Helper()

	if err := os.Chtimes(
		filepath.Join(root, ".jj", "repo", "op_heads", "heads"),
		at,
		at,
	); err != nil {
		t.Fatal(err)
	}
}

// counting answers with out and counts how often it was asked.
func counting(out string, err error) (Runner, *int) {
	calls := 0

	return func(context.Context, string) (string, error) {
		calls++

		return out, err
	}, &calls
}

func TestALabelIsReadAgainOnlyAfterAnOperation(t *testing.T) {
	t.Parallel()

	root := workspace(t)
	touch(t, root, time.Unix(1000, 0))

	run, calls := counting("xkurvtuq (main~1)", nil)
	reader := NewWithRunner(run)

	for range 3 {
		if got := reader.Label(t.Context(), root); got != "main~1" {
			t.Fatalf("Label = %q, want main~1", got)
		}
	}

	if *calls != 1 {
		t.Errorf("runs = %d before any operation, want 1", *calls)
	}

	touch(t, root, time.Unix(2000, 0))
	reader.Label(t.Context(), root)

	if *calls != 2 {
		t.Errorf("runs = %d after an operation, want 2", *calls)
	}
}

func TestAFailedRunIsNotRemembered(t *testing.T) {
	t.Parallel()

	root := workspace(t)
	run, calls := counting("", errors.New("exit status 1"))
	reader := NewWithRunner(run)

	for range 2 {
		if got := reader.Label(t.Context(), root); got != "" {
			t.Fatalf("Label = %q, want nothing", got)
		}
	}

	if *calls != 2 {
		t.Errorf("runs = %d, want each failure retried", *calls)
	}
}

func TestARunIsBoundedByATimeout(t *testing.T) {
	t.Parallel()

	root := workspace(t)
	reader := NewWithRunner(func(ctx context.Context, _ string) (string, error) {
		if _, bounded := ctx.Deadline(); !bounded {
			t.Error("run carries no deadline")
		}

		return "xkurvtuq", nil
	})

	reader.Label(t.Context(), root)
}

func TestNoRunnerLabelsNothing(t *testing.T) {
	t.Parallel()

	if got := NewWithRunner(nil).Label(t.Context(), workspace(t)); got != "" {
		t.Errorf("Label = %q, want nothing without jj-starship", got)
	}
}

func TestADirectoryWithoutARepositoryIsNotRun(t *testing.T) {
	t.Parallel()

	run, calls := counting("xkurvtuq", nil)
	if got := NewWithRunner(run).Label(t.Context(), t.TempDir()); got != "" || *calls != 0 {
		t.Errorf("Label = %q after %d runs, want nothing and no run", got, *calls)
	}
}

func TestASecondWorkspaceReadsTheSharedOperationLog(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	main := filepath.Join(parent, "main")
	second := filepath.Join(parent, "second")

	if err := os.MkdirAll(
		filepath.Join(main, ".jj", "repo", "op_heads", "heads"),
		0o755,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(second, ".jj"), 0o755); err != nil {
		t.Fatal(err)
	}

	// What `jj workspace add` writes: relative to the workspace's .jj.
	if err := os.WriteFile(
		filepath.Join(second, ".jj", "repo"),
		[]byte("../../main/.jj/repo"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	run, _ := counting("ymznnlvz (demo~2)", nil)
	if got := NewWithRunner(run).Label(t.Context(), second); got != "demo~2" {
		t.Errorf("Label = %q, want demo~2 through the shared repository", got)
	}
}

func TestRootIsTheNearestWorkspace(t *testing.T) {
	t.Parallel()

	root := workspace(t)
	nested := filepath.Join(root, "a", "b")

	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	if got, ok := Root(nested); !ok || got != root {
		t.Errorf("Root(%q) = %q, %v, want %q", nested, got, ok, root)
	}

	if _, ok := Root(t.TempDir()); ok {
		t.Error("Root found a workspace where there is none")
	}

	if _, ok := Root("relative/dir"); ok {
		t.Error("Root accepted a relative directory")
	}
}
