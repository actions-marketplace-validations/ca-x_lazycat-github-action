package platformapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIHost        = "appstore.api.lazycat.cloud"
	defaultCOSDomain      = "dl.lazycat.cloud"
	invalidAPIHostBaseURL = "https://invalid-lzc-api-host.invalid"
)

func Host() (string, error) {
	return ResolveHost(os.Getenv("LZC_API_HOST"))
}

func ResolveHost(configured string) (string, error) {
	host := strings.TrimSpace(configured)
	if host == "" {
		host = defaultAPIHost
	}
	if strings.Contains(host, "://") || strings.ContainsAny(host, "/?#") || strings.ContainsAny(host, " \t\r\n") {
		return "", errors.New("LZC_API_HOST must contain only a host name")
	}
	return host, nil
}

func BaseURL() string {
	host, err := Host()
	if err != nil {
		return invalidAPIHostBaseURL
	}
	return "https://" + host
}

func ResolveBaseURL(configured string) (string, error) {
	host, err := ResolveHost(configured)
	if err != nil {
		return "", err
	}
	return "https://" + host, nil
}

func AppStoreCOSBaseURL() (string, error) {
	domain := strings.TrimSpace(os.Getenv("LZC_APPSTORE_COS_DOMAIN"))
	if domain == "" {
		domain = defaultCOSDomain
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, ":/?#") || strings.ContainsAny(domain, " \t\r\n") {
		return "", errors.New("LZC_APPSTORE_COS_DOMAIN must contain only a domain name")
	}
	return "https://" + domain + "/appstore/metarepo", nil
}

func HTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{Timeout: 30 * time.Second}
	}
	cloned := *base
	transport := cloned.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	cloned.Transport = sdkTransport{base: transport}
	cloned.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &cloned
}

type sdkTransport struct {
	base http.RoundTripper
}

func (transport sdkTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	if strings.HasPrefix(cloned.URL.Path, "/api/v3/developer") {
		urlCopy := *cloned.URL
		urlCopy.Path = "/sdk/v3/developer" + strings.TrimPrefix(cloned.URL.Path, "/api/v3/developer")
		cloned.URL = &urlCopy
	}
	token := strings.TrimSpace(cloned.Header.Get("X-User-Token"))
	cloned.Header.Del("X-User-Token")
	cloned.Header.Del("Cookie")
	if token != "" {
		cloned.Header.Set("X-API-Token", token)
	}
	response, err := transport.base.RoundTrip(cloned)
	if err != nil || response == nil || !strings.HasPrefix(cloned.URL.Path, "/sdk/v3/developer") {
		return response, err
	}
	return unwrapSDKResponse(response)
}

func unwrapSDKResponse(response *http.Response) (*http.Response, error) {
	body, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		response.Body = io.NopCloser(bytes.NewReader(body))
		return response, nil
	}
	rawCode, found := envelope["errorCode"]
	if !found {
		response.Body = io.NopCloser(bytes.NewReader(body))
		return response, nil
	}
	var errorCode int
	if json.Unmarshal(rawCode, &errorCode) != nil {
		response.Body = io.NopCloser(bytes.NewReader(body))
		return response, nil
	}
	payload := envelope["data"]
	if errorCode != 0 {
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			response.StatusCode = http.StatusBadRequest
			response.Status = "400 Bad Request"
		}
		var message string
		_ = json.Unmarshal(envelope["msg"], &message)
		payload, _ = json.Marshal(map[string]string{"message": strings.TrimSpace(message)})
	} else if len(payload) == 0 || bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		payload = []byte("{}")
	}
	response.Body = io.NopCloser(bytes.NewReader(payload))
	response.ContentLength = int64(len(payload))
	response.Header.Set("Content-Length", strconv.Itoa(len(payload)))
	return response, nil
}
