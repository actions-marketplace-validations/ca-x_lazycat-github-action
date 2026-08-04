package platformauth

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/ca-x/lazycat-github-action/internal/platformapi"
	lpkgo "github.com/lib-x/lzc-toolkit-go"
	"github.com/lib-x/lzc-toolkit-go/auth"
)

type Result struct {
	Provider auth.TokenProvider
}

type Resolver struct {
	LookupEnv func(string) (string, bool)
}

func (resolver Resolver) Resolve(ctx context.Context) (Result, error) {
	if ctx == nil {
		return Result{}, authError(lpkgo.CodeInvalidArgument, errors.New("context is required"))
	}
	if err := ctx.Err(); err != nil {
		return Result{}, authError(lpkgo.CodeCancelled, err)
	}
	if _, err := platformapi.Host(); err != nil {
		return Result{}, authError(lpkgo.CodeInvalidArgument, err)
	}
	lookup := resolver.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	token, _ := lookup("LZC_API_TOKEN")
	token = strings.TrimSpace(token)
	if token == "" {
		return Result{}, authError(lpkgo.CodeUnauthenticated, errors.New("LZC_API_TOKEN is required"))
	}
	return Result{Provider: auth.StaticToken(token)}, nil
}

func authError(code lpkgo.Code, cause error) error {
	return &lpkgo.Error{Code: code, Op: "platformauth.resolve", Cause: cause}
}
