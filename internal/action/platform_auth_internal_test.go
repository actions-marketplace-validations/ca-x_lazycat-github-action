package action

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ca-x/lazycat-github-action/internal/platformauth"
	lpkgo "github.com/lib-x/lzc-toolkit-go"
	"github.com/lib-x/lzc-toolkit-go/appstore"
	"github.com/lib-x/lzc-toolkit-go/auth"
)

func TestPlatformStoreOptionsSelectAuthenticationProtocol(t *testing.T) {
	t.Setenv("LZC_API_HOST", "")
	provider := auth.StaticToken("token")

	pat := platformStoreOptions(platformauth.Result{
		Protocol: platformauth.ProtocolPAT, Provider: provider, BaseURL: "https://appstore.api.lazycat.cloud",
	})
	if pat.BaseURL != "https://appstore.api.lazycat.cloud" || pat.HTTPClient == nil || pat.Token == nil {
		t.Fatalf("PAT options=%#v", pat)
	}

	legacy := platformStoreOptions(platformauth.Result{Protocol: platformauth.ProtocolLegacySession, Provider: provider})
	if legacy.BaseURL != "" || legacy.HTTPClient == nil || legacy.Token == nil {
		t.Fatalf("legacy options=%#v", legacy)
	}
}

func TestPlatformImageCopierReusesFirstAuthenticationSnapshot(t *testing.T) {
	values := map[string]string{"LAZYCAT_TOKEN": "legacy-session"}
	lookups := 0
	copier := &platformImageCopier{resolver: platformauth.Resolver{LookupEnv: func(name string) (string, bool) {
		lookups++
		value, found := values[name]
		return value, found
	}}}
	first, err := copier.clientFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstLookups := lookups
	values = map[string]string{"LZC_API_TOKEN": "pat", "LZC_API_HOST": "new.example.invalid"}
	second, err := copier.clientFor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || lookups != firstLookups {
		t.Fatalf("client reused=%t lookups=%d want=%d", first == second, lookups, firstLookups)
	}
}

func TestLegacyImageCopyDoesNotForwardSessionAcrossRedirect(t *testing.T) {
	reachedTarget := false
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		reachedTarget = true
		if request.Header.Get("X-User-Token") != "" || request.Header.Get("Cookie") != "" {
			t.Error("redirect target received legacy session credentials")
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-User-Token") != "legacy-session" {
			t.Errorf("origin X-User-Token=%q", request.Header.Get("X-User-Token"))
		}
		if cookie, err := request.Cookie("userToken"); err != nil || cookie.Value != "legacy-session" {
			t.Errorf("origin cookie=%v err=%v", cookie, err)
		}
		http.Redirect(response, request, target.URL+request.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	options := platformStoreOptions(platformauth.Result{
		Protocol: platformauth.ProtocolLegacySession,
		Provider: auth.StaticToken("legacy-session"),
	})
	options.BaseURL = origin.URL
	_, err := appstore.New(options).CopyImage(context.Background(), appstore.CopyImageRequest{
		Image: "docker.io/library/alpine:latest", Platform: "amd64",
	})
	if err == nil || !errors.Is(err, lpkgo.ErrRemoteUnavailable) {
		t.Fatalf("err=%v", err)
	}
	if reachedTarget {
		t.Fatal("legacy image-copy client followed redirect")
	}
}
