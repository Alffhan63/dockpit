package hosts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"dockpit/server/internal/auth"
	"dockpit/server/internal/storage"
)

// Register adds a host named name and returns it with its new agent token.
// The token is returned only here; only its hash is stored.
func Register(ctx context.Context, store *storage.Store, name string) (storage.Host, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return storage.Host{}, "", errors.New("name must be 1-64 characters")
	}
	id, err := uniqueID(ctx, store, Slug(name))
	if err != nil {
		return storage.Host{}, "", err
	}
	token := auth.NewToken()
	h, err := store.CreateHost(ctx, id, name, auth.HashToken(token))
	return h, token, err
}

// RotateToken issues a new token for a host, invalidating the old one.
func RotateToken(ctx context.Context, store *storage.Store, id string) (string, error) {
	token := auth.NewToken()
	if err := store.SetHostToken(ctx, id, auth.HashToken(token)); err != nil {
		return "", err
	}
	return token, nil
}

func uniqueID(ctx context.Context, store *storage.Store, base string) (string, error) {
	for i := 1; i <= 100; i++ {
		id := base
		if i > 1 {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		taken, err := store.HostExists(ctx, id)
		if err != nil {
			return "", err
		}
		if !taken {
			return id, nil
		}
	}
	return "", errors.New("too many hosts with this name")
}
