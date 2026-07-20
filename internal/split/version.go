package split

import (
	"context"
	"strings"
	"time"
)

// versionTimeout bounds the shntool version probe. A banner print is
// instantaneous; the timeout only guards against a wedged binary holding
// up server startup.
const versionTimeout = 5 * time.Second

// ShntoolVersion reports the version of the installed shntool — the tool
// behind shnsplit, which does the actual splitting — as printed by its
// `shntool -v` banner (e.g. "3.0.10"). It returns "" when shntool is
// missing or its banner is unrecognizable: a version probe is
// informational, so an absent tool is reported as an absent version
// rather than as an error.
func ShntoolVersion(ctx context.Context) string {
	// The exit status is deliberately ignored: some shntool builds report
	// their version and exit non-zero, and the banner is all we need.
	out, _ := runContext(ctx, versionTimeout, "shntool", "-v")

	banner, _, _ := strings.Cut(out, "\n")
	fields := strings.Fields(banner)
	if len(fields) < 2 || fields[0] != "shntool" {
		return ""
	}
	return fields[1]
}
