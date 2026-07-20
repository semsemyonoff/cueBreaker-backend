package split

import (
	"context"
	"testing"
)

func TestShntoolVersion(t *testing.T) {
	cases := []struct {
		name   string
		script string // empty means: do not install a fake shntool at all
		want   string
	}{
		{
			name:   "real banner",
			script: "echo 'shntool 3.0.10'\necho 'Copyright (C) 2000-2009 Jason Jordan'\n",
			want:   "3.0.10",
		},
		{
			// Some builds print the banner and exit non-zero; the version is
			// still there and must still be reported.
			name:   "banner with non-zero exit",
			script: "echo 'shntool 3.0.10'\nexit 1\n",
			want:   "3.0.10",
		},
		{
			name:   "unrecognized banner",
			script: "echo 'some other tool'\n",
			want:   "",
		},
		{
			name:   "empty output",
			script: "exit 0\n",
			want:   "",
		},
		{
			name:   "tool missing",
			script: "",
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// An empty PATH dir both installs the fake in isolation and, when
			// no fake is written, hides any shntool the host may have.
			dir := t.TempDir()
			if tc.script != "" {
				writeFakeTool(t, dir, "shntool", tc.script)
			}
			t.Setenv("PATH", dir)

			if got := ShntoolVersion(context.Background()); got != tc.want {
				t.Fatalf("ShntoolVersion() = %q, want %q", got, tc.want)
			}
		})
	}
}
