package monobank

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

// sdkVersion is the sentinel returned when debug.ReadBuildInfo cannot
// resolve the real version (for example, tests or a replaced module).
const sdkVersion = "(devel)"

// userAgentPrefix is the User-Agent part before any user-supplied
// prefix. Format: "go-monobank-sdk/vX.Y.Z (linux; go1.26.2)". We look
// up the version once and cache it — debug.ReadBuildInfo walks the
// linker tables and is not free.
var userAgentPrefix = sync.OnceValue(func() string {
	v := sdkVersion
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/OlexiyOdarchuk/go-monobank-sdk" {
				v = dep.Version
				break
			}
		}
	}
	return fmt.Sprintf("go-monobank-sdk/%s (%s; %s)", v, runtime.GOOS, runtime.Version())
})

// UserAgent returns the User-Agent the SDK uses by default. Exported
// so you can compose your own value for [WithUserAgent] without
// losing the SDK portion:
//
//	cli := personal.New(token,
//	    monobank.WithUserAgent("myapp/1.2.3 "+monobank.UserAgent()),
//	)
func UserAgent() string { return userAgentPrefix() }
