//go:build !enterprise

package api

import (
	"net/http"

	"github.com/PrivateCaptcha/PrivateCaptcha/pkg/common"
	"github.com/justinas/alice"
)

func (s *Server) setupEnterprise(rg *common.RouteGenerator, publicChain alice.Chain, apiRateLimiter func(next http.Handler) http.Handler) {
}
