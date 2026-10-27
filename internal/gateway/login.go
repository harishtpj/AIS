package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/harishtpj/ais/internal/auth"
	"github.com/harishtpj/ais/internal/responses"
	"github.com/harishtpj/ais/internal/service"
)

func (g *Gateway) handleLogin(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "auth" || parts[2] != "login" {
		responses.WriteError(w, http.StatusNotFound, "not_found", "The requested endpoint was not found.")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		responses.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST requests are supported.")
		return
	}

	svc, err := g.registry.Get(parts[1])
	if err != nil || svc.AuthStrategy == service.AuthNone {
		responses.WriteError(w, http.StatusNotFound, "not_found", "The requested endpoint was not found.")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxLoginBody+1))
	if err != nil || len(body) > maxLoginBody {
		responses.WriteError(w, http.StatusBadRequest, "invalid_request", "The login request body is invalid.")
		return
	}
	var loginInput map[string]any
	if err := json.Unmarshal(body, &loginInput); err != nil {
		responses.WriteError(w, http.StatusBadRequest, "invalid_request", "The login request body must be valid JSON.")
		return
	}
	subject := firstString(loginInput, "username", "email", "user", "sub")

	baseURL := svc.TargetURL()
	if baseURL == nil {
		responses.WriteError(w, http.StatusBadGateway, "backend_unavailable", "The authentication backend is unavailable.")
		return
	}
	target := *baseURL
	target.Path = path.Join(strings.TrimRight(baseURL.Path, "/"), svc.AuthPath)
	target.RawQuery = r.URL.RawQuery
	backendReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		responses.WriteError(w, http.StatusBadGateway, "backend_request_failed", "The authentication request could not be created.")
		return
	}
	for name, values := range r.Header {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Host") {
			continue
		}
		for _, value := range values {
			backendReq.Header.Add(name, value)
		}
	}

	resp, err := g.httpClient.Do(backendReq)
	if err != nil {
		responses.WriteError(w, http.StatusBadGateway, "backend_unavailable", "The authentication backend is unavailable.")
		return
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxLoginBody+1))
	if readErr != nil || len(responseBody) > maxLoginBody {
		responses.WriteError(w, http.StatusBadGateway, "invalid_backend_response", "The authentication backend returned an invalid response.")
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if contentType := resp.Header.Get("Content-Type"); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(responseBody)
		return
	}

	credential := ""
	switch svc.AuthStrategy {
	case service.AuthSessions:
		pairs := make([]string, 0, len(resp.Cookies()))
		for _, cookie := range resp.Cookies() {
			pairs = append(pairs, cookie.Name+"="+cookie.Value)
		}
		credential = strings.Join(pairs, "; ")
	case service.AuthOpaque:
		var payload map[string]any
		if err := json.Unmarshal(responseBody, &payload); err == nil {
			credential = firstString(payload, "access_token")
		}
	default:
		responses.WriteError(w, http.StatusBadGateway, "unsupported_auth_strategy", "The backend authentication strategy is unsupported.")
		return
	}
	if credential == "" {
		responses.WriteError(w, http.StatusBadGateway, "missing_backend_credential", "The authentication backend returned no usable credential.")
		return
	}
	if subject == "" {
		var payload map[string]any
		if json.Unmarshal(responseBody, &payload) == nil {
			subject = firstString(payload, "username", "email", "sub", "user_id")
		}
	}
	if subject == "" {
		responses.WriteError(w, http.StatusBadGateway, "missing_subject", "The login payload or response must identify the authenticated user.")
		return
	}

	token, err := g.authManager.CreateSession(r.Context(), auth.Session{
		Service: svc.Name, Subject: subject, Strategy: svc.AuthStrategy, Credential: credential,
	})
	if err != nil {
		g.logger.Error().Err(err).Str("service", svc.Name).Msg("could not create auth session")
		responses.WriteError(w, http.StatusBadGateway, "session_creation_failed", "The gateway session could not be created.")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   900,
	})
}

func (g *Gateway) requestWithBackendCredential(r *http.Request, svc *service.Service) (*http.Request, error) {
	header := strings.Fields(r.Header.Get("Authorization"))
	if len(header) != 2 || !strings.EqualFold(header[0], "Bearer") {
		return nil, errors.New("missing bearer JWT")
	}
	session, err := g.authManager.Resolve(r.Context(), header[1], svc.Name)
	if err != nil || session.Strategy != svc.AuthStrategy {
		return nil, errors.New("invalid JWT session")
	}

	forwarded := r.Clone(r.Context())
	forwarded.Header = r.Header.Clone()
	forwarded.Header.Del("Authorization")
	forwarded.Header.Del("Cookie")
	if session.Strategy == service.AuthSessions {
		forwarded.Header.Set("Cookie", session.Credential)
	} else if session.Strategy == service.AuthOpaque {
		forwarded.Header.Set("Authorization", "Bearer "+session.Credential)
	} else {
		return nil, errors.New("unsupported backend auth strategy")
	}
	return forwarded, nil
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
