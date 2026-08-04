package platformapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ca-x/lazycat-github-action/internal/platformapi"
)

func TestBaseURLReadsLZCAPIHost(t *testing.T) {
	t.Setenv("LZC_API_HOST", "appstore-api.staging.lazycat.cloud")
	if got := platformapi.BaseURL(); got != "https://appstore-api.staging.lazycat.cloud" {
		t.Fatalf("base URL=%q", got)
	}
}

func TestAppStoreCOSBaseURLReadsEnvironment(t *testing.T) {
	t.Setenv("LZC_APPSTORE_COS_DOMAIN", "lzc-app-staging-1301583638.cos.ap-guangzhou.myqcloud.com")
	got, err := platformapi.AppStoreCOSBaseURL()
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://lzc-app-staging-1301583638.cos.ap-guangzhou.myqcloud.com/appstore/metarepo" {
		t.Fatalf("COS URL=%q", got)
	}
}

func TestAppStoreCOSBaseURLRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{
		"http://cos.example.com/appstore/metarepo",
		"cos.example.com:443",
		"cos.example.com/appstore/metarepo",
		"cos.example.com?token=secret",
		"cos.example.com#fragment",
		"cos example.com",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LZC_APPSTORE_COS_DOMAIN", value)
			if _, err := platformapi.AppStoreCOSBaseURL(); err == nil {
				t.Fatal("expected unsafe COS URL to fail")
			}
		})
	}
}

func TestHTTPClientRewritesSDKPathAndAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sdk/v3/developer/app/lpk/upload" {
			t.Errorf("path=%q", request.URL.Path)
		}
		if request.Header.Get("X-API-Token") != "pat-token" {
			t.Errorf("X-API-Token=%q", request.Header.Get("X-API-Token"))
		}
		if request.Header.Get("X-User-Token") != "" {
			t.Errorf("X-User-Token must be empty")
		}
		if request.Header.Get("Cookie") != "" {
			t.Errorf("Cookie must be empty")
		}
		_, _ = io.WriteString(response, "{\"errorCode\":0,\"msg\":\"ok\",\"data\":{\"exist\":true}}")
	}))
	defer server.Close()

	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v3/developer/app/lpk/upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-User-Token", "pat-token")
	request.Header.Set("Cookie", "userToken=pat-token")
	response, err := platformapi.HTTPClient(server.Client()).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if string(body) != "{\"exist\":true}" {
		t.Fatalf("body=%s", body)
	}
}

func TestHTTPClientDoesNotForwardPATAcrossRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		reached = true
		if request.Header.Get("X-API-Token") != "" || request.Header.Get("X-User-Token") != "" || request.Header.Get("Cookie") != "" {
			t.Error("redirect target received credentials")
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		http.Redirect(response, request, target.URL+"/sdk/v3/developer/app/lpk/upload", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	request, err := http.NewRequest(http.MethodPost, origin.URL+"/api/v3/developer/app/lpk/upload", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-User-Token", "pat-token")
	request.Header.Set("Cookie", "userToken=pat-token")
	response, err := platformapi.HTTPClient(origin.Client()).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || reached {
		t.Fatalf("status=%d reached=%t", response.StatusCode, reached)
	}
}
