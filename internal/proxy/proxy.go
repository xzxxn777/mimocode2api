package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Bootstrap response
type bootstrapResponse struct {
	JWT string `json:"jwt"`
}

// JWT cache
type jwtCache struct {
	mu  sync.Mutex
	jwt string
	exp int64
}

var cache jwtCache

// GenerateFingerprint creates a device fingerprint matching MiMo-Code's implementation.
// SHA256 of: hostname|platform|arch|cpu_model|username
func GenerateFingerprint() string {
	hostname, _ := os.Hostname()
	cpu := "unknown-cpu"
	username := "unknown-user"

	if u, err := os.UserHomeDir(); err == nil {
		// Extract username from home dir
		parts := strings.Split(u, "/")
		if len(parts) > 0 {
			username = parts[len(parts)-1]
		}
	}

	seed := fmt.Sprintf("%s|%s|%s|%s|%s", hostname, runtime.GOOS, runtime.GOARCH, cpu, username)
	hash := sha256.Sum256([]byte(seed))
	return fmt.Sprintf("%x", hash)
}

// parseJWTExp extracts the exp claim from a JWT
func parseJWTExp(jwt string) int64 {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return time.Now().Add(50 * time.Minute).UnixMilli()
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Now().Add(50 * time.Minute).UnixMilli()
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return time.Now().Add(50 * time.Minute).UnixMilli()
	}
	return claims.Exp * 1000 // convert to milliseconds
}

// Bootstrap exchanges device fingerprint for JWT
func Bootstrap(bootstrapURL, fingerprint string) (string, error) {
	body, _ := json.Marshal(map[string]string{"client": fingerprint})
	resp, err := http.Post(bootstrapURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("bootstrap: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return "", fmt.Errorf("bootstrap: %d %s", resp.StatusCode, string(b))
	}

	var result bootstrapResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("bootstrap decode: %w", err)
	}
	if result.JWT == "" {
		return "", fmt.Errorf("bootstrap: no jwt in response")
	}
	return result.JWT, nil
}

const jwtRefreshBuffer = 5 * time.Minute

// GetJWT returns a valid JWT, refreshing if needed
func GetJWT(bootstrapURL, fingerprint string) (string, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	if cache.jwt != "" && cache.exp-time.Now().UnixMilli() > jwtRefreshBuffer.Milliseconds() {
		return cache.jwt, nil
	}

	jwt, err := Bootstrap(bootstrapURL, fingerprint)
	if err != nil {
		if cache.jwt != "" {
			log.Printf("[JWT] Bootstrap failed, using cached JWT: %v", err)
			return cache.jwt, nil
		}
		return "", err
	}

	cache.jwt = jwt
	cache.exp = parseJWTExp(jwt)
	log.Printf("[JWT] Bootstrapped, exp in %v", time.Until(time.UnixMilli(cache.exp)).Round(time.Second))
	return jwt, nil
}

// ProxyHandler forwards requests to the upstream MiMo API with JWT auth
func ProxyHandler(chatURL, bootstrapURL, fingerprint string) http.HandlerFunc {
	client := &http.Client{
		Timeout: 300 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 20,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Get JWT
		jwt, err := GetJWT(bootstrapURL, fingerprint)
		if err != nil {
			http.Error(w, `{"error":{"message":"JWT bootstrap failed: `+err.Error()+`"}}`, http.StatusBadGateway)
			return
		}

		// Read and rewrite request body (strip provider/ prefix from model)
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error":{"message":"Failed to read body"}}`, http.StatusBadRequest)
			return
		}
		r.Body.Close()

		body := rewriteModelField(rawBody)

		// Create upstream request
		upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", chatURL, bytes.NewReader(body))
		if err != nil {
			http.Error(w, `{"error":{"message":"Upstream error"}}`, http.StatusInternalServerError)
			return
		}

		// Copy relevant headers
		upstreamReq.Header.Set("Content-Type", "application/json")
		upstreamReq.Header.Set("Authorization", "Bearer "+jwt)
		upstreamReq.Header.Set("X-Mimo-Source", "mimocode-cli-free")
		upstreamReq.Header.Set("Accept", "text/event-stream, application/json")

		// Random user-agent rotation
		upstreamReq.Header.Set("User-Agent", randomUA())

		log.Printf("[Proxy] Sending to upstream, body=%d bytes", len(body))

		// Make request
		resp, err := client.Do(upstreamReq)
		if err != nil {
			// Retry once with fresh JWT on 401/403
			if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "403") {
				cache.mu.Lock()
				cache.jwt = ""
				cache.mu.Unlock()
				jwt, err = GetJWT(bootstrapURL, fingerprint)
				if err != nil {
					http.Error(w, `{"error":{"message":"JWT refresh failed"}}`, http.StatusBadGateway)
					return
				}
				upstreamReq.Header.Set("Authorization", "Bearer "+jwt)
				resp, err = client.Do(upstreamReq)
				if err != nil {
					http.Error(w, `{"error":{"message":"Upstream error: `+err.Error()+`"}}`, http.StatusBadGateway)
					return
				}
			} else {
				http.Error(w, `{"error":{"message":"Upstream error: `+err.Error()+`"}}`, http.StatusBadGateway)
				return
			}
		}
		defer resp.Body.Close()

		// On 401/403, retry with fresh JWT
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			cache.mu.Lock()
			cache.jwt = ""
			cache.mu.Unlock()
			jwt, err = GetJWT(bootstrapURL, fingerprint)
			if err != nil {
				http.Error(w, `{"error":{"message":"JWT refresh failed"}}`, http.StatusBadGateway)
				return
			}

			upstreamReq.Body = io.NopCloser(bytes.NewReader(body))
			upstreamReq.Header.Set("Authorization", "Bearer "+jwt)
			resp2, err := client.Do(upstreamReq)
			if err != nil {
				http.Error(w, `{"error":{"message":"Upstream error: `+err.Error()+`"}}`, http.StatusBadGateway)
				return
			}
			resp.Body.Close()
			resp = resp2
		}

		// Copy response headers
		for key, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(key, v)
			}
		}

		// Check if this is a non-streaming response (upstream returns SSE with "data:" prefix)
		contentType := resp.Header.Get("Content-Type")
		isStream := strings.Contains(contentType, "text/event-stream")

		if isStream {
			// For streaming: just pass through as-is
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
		} else {
			// For non-streaming: strip "data:" prefix and return pure JSON
			respBody, _ := io.ReadAll(resp.Body)
			bodyStr := string(respBody)
			// Strip "data:" prefix
			if strings.HasPrefix(bodyStr, "data:") {
				bodyStr = strings.TrimPrefix(bodyStr, "data:")
				bodyStr = strings.TrimSpace(bodyStr)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			w.Write([]byte(bodyStr))
		}
	}
}

var uaList = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
}

func randomUA() string {
	return uaList[rand.Intn(len(uaList))]
}

// rewriteModelField strips the "provider/" prefix from the model field.
// "mimo/mimo-auto" → "mimo-auto"
func rewriteModelField(body []byte) []byte {
	// Find "model" key and rewrite its value
	// Simple JSON surgery: find \"model\":\"...\" and strip the provider/ prefix
	idx := 0
	for {
		// Find "model":
		pos := bytes.Index(body[idx:], []byte(`"model"`))
		if pos < 0 {
			break
		}
		pos += idx
		// Skip past "model":"
		start := pos + len(`"model"`)
		// Skip whitespace and colon
		for start < len(body) && (body[start] == ' ' || body[start] == '	' || body[start] == ':' || body[start] == '\n' || body[start] == '\r') {
			start++
		}
		// Find the value start
		if start >= len(body) || body[start] != '"' {
			idx = start
			continue
		}
		start++ // skip opening quote
		// Find the value end
		end := start
		for end < len(body) && body[end] != '"' {
			end++
		}
		if end <= start {
			idx = end
			continue
		}
		modelValue := string(body[start:end])
		// Strip provider/ prefix
		if slashIdx := strings.LastIndex(modelValue, "/"); slashIdx >= 0 {
			newModel := modelValue[slashIdx+1:]
			if newModel != modelValue {
				newBody := make([]byte, 0, len(body)-(len(modelValue)-len(newModel)))
				newBody = append(newBody, body[:start]...)
				newBody = append(newBody, []byte(newModel)...)
				newBody = append(newBody, body[end:]...)
				body = newBody
			}
		}
		idx = end
	}
	return body
}