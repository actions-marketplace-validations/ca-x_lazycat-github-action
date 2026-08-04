package platformauth_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ca-x/lazycat-github-action/internal/platformauth"
	lpkgo "github.com/lib-x/lzc-toolkit-go"
)

func TestResolverUsesOnlyLZCAPIToken(t *testing.T) {
	t.Setenv("LZC_API_HOST", "")
	values := map[string]string{"LZC_API_TOKEN": " pat-token "}
	result, err := (platformauth.Resolver{LookupEnv: func(name string) (string, bool) {
		value, found := values[name]
		return value, found
	}}).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	token, err := result.Provider.Token(context.Background())
	if err != nil || token != "pat-token" {
		t.Fatalf("token=%q err=%v", token, err)
	}
}

func TestResolverRequiresLZCAPIToken(t *testing.T) {
	t.Setenv("LZC_API_HOST", "")
	_, err := (platformauth.Resolver{LookupEnv: func(string) (string, bool) {
		return "", false
	}}).Resolve(context.Background())
	if err == nil || !errors.Is(err, lpkgo.ErrUnauthenticated) {
		t.Fatalf("err=%v", err)
	}
}

func TestProviderCachesLZCAPIToken(t *testing.T) {
	t.Setenv("LZC_API_HOST", "")
	var lookups atomic.Int64
	provider := platformauth.NewProvider(platformauth.Resolver{
		LookupEnv: func(name string) (string, bool) {
			if name == "LZC_API_TOKEN" {
				lookups.Add(1)
				return "pat-token", true
			}
			return "", false
		},
	})

	start := make(chan struct{})
	results := make(chan string, 16)
	errors := make(chan error, 16)
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			token, err := provider.Token(context.Background())
			results <- token
			errors <- err
		}()
	}
	close(start)
	group.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	for token := range results {
		if token != "pat-token" {
			t.Fatalf("token=%q", token)
		}
	}
	if lookups.Load() != 1 {
		t.Fatalf("lookups=%d", lookups.Load())
	}
}
