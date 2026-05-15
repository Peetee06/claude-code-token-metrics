package repo

import "testing"

func TestNormalizeWorktreePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "squad worktree with branch segment and hash suffix",
			in:   "/Users/me/.claude-squad/worktrees/fix/drift-migrations_18afaafbb0011223",
			want: "fix/drift-migrations",
		},
		{
			name: "squad worktree single-segment branch",
			in:   "/Users/me/.claude-squad/worktrees/build-ast-tool_18af8b8b8eb18d58",
			want: "build-ast-tool",
		},
		{
			name: "non-squad path returns empty",
			in:   "/Users/me/dev/Iakuvo",
			want: "",
		},
		{
			name: "empty input",
			in:   "",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeWorktreeBranch(c.in); got != c.want {
				t.Errorf("NormalizeWorktreeBranch(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
