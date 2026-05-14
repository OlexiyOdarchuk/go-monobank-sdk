package monobank

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
)

// sdkVersion — sentinel, який вертається коли debug.ReadBuildInfo не
// може дістати реальну версію (наприклад, тести або replaced module).
const sdkVersion = "(devel)"

// userAgentPrefix — частина User-Agent до користувацького префіксу.
// Формат: "go-monobank-sdk/vX.Y.Z (linux; go1.26.2)". Лазимо за версією
// раз і кешуємо — debug.ReadBuildInfo робить linker-обхід, не безкоштовно.
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

// UserAgent повертає User-Agent, який SDK ставить за замовчуванням.
// Експортовано на випадок, якщо ти хочеш скласти власне значення для
// [WithUserAgent], не втрачаючи SDK-частини:
//
//	cli := personal.New(token,
//	    monobank.WithUserAgent("myapp/1.2.3 "+monobank.UserAgent()),
//	)
func UserAgent() string { return userAgentPrefix() }
