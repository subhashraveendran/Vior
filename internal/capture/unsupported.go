package capture

import "errors"

// ErrUnsupported is returned by the display-arrangement operations
// (MirrorDisplay, UnmirrorDisplay) on platforms where Vior cannot change
// the OS display layout — today everything except macOS. Callers that only
// need the virtual display to exist (session.Configure) treat it as a
// warning; user-facing commands turn it into "not available on this
// platform" rather than a failure.
var ErrUnsupported = errors.New("display arrangement not supported on this platform")
