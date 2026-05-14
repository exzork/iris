package browser

import (
	"net/url"
	"strings"

	loresource "github.com/eko/iris-bot/internal/lore/source"
)

// Gate enforces compliance checks before allowing browser lookups.
// It consults the source registry to verify that a host is registered
// and that the browser access method is allowed by that host's policy.
type Gate struct {
	Registry *loresource.Registry
}

// Allow returns nil if the given URL's host is registered and browser method
// is allowed. Otherwise it returns ErrHostNotRegistered or ErrMethodNotAllowed.
func (g *Gate) Allow(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	host := strings.ToLower(strings.TrimSpace(u.Host))
	if host == "" {
		return ErrHostNotRegistered
	}

	// Check if host is registered
	source, ok := g.Registry.ByHost(host)
	if !ok {
		return ErrHostNotRegistered
	}

	// Check if browser method is allowed
	if !source.Policy.Allows(loresource.MethodBrowser) {
		return ErrMethodNotAllowed
	}

	return nil
}
