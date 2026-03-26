package router

import (
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func SetClawProxyRouter(router *gin.Engine, targetBaseURL string) {
	targetBaseURL = strings.TrimSpace(targetBaseURL)
	if targetBaseURL == "" {
		targetBaseURL = "http://127.0.0.1:3000"
	}
	targetBaseURL = strings.TrimSuffix(targetBaseURL, "/")

	targetURL, err := url.Parse(targetBaseURL)
	if err != nil {
		common.SysError("invalid POCO_CLAW_FRONTEND_URL: " + err.Error())
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalHost := req.Host
		originalDirector(req)
		req.Host = targetURL.Host
		if req.Header.Get("X-Forwarded-Host") == "" && originalHost != "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		}
		if req.Header.Get("X-Forwarded-Proto") == "" {
			if req.TLS != nil {
				req.Header.Set("X-Forwarded-Proto", "https")
			} else {
				req.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		if clientIP, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr)); err == nil && clientIP != "" {
			req.Header.Set("X-Forwarded-For", clientIP)
		} else if clientIP := strings.TrimSpace(req.RemoteAddr); clientIP != "" {
			req.Header.Set("X-Forwarded-For", clientIP)
		}
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = rw.Write([]byte(`{"success":false,"message":"claw frontend unavailable: ` + err.Error() + `"}`))
	}

	handler := func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	}

	clawRoute := router.Group("")
	clawRoute.Use(middleware.TokenOrUserAuth())
	clawRoute.Any("/claw", handler)
	clawRoute.Any("/claw/*path", handler)
}
