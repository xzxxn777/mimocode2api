package handler

import (
	"encoding/json"
	"net/http"

	"github.com/akino/reverse-mimocode/internal/model"
	"github.com/akino/reverse-mimocode/internal/proxy"
)

type ProxyConfig struct {
	ChatURL      string
	BootstrapURL string
	Fingerprint  string
}

func Models() http.HandlerFunc {
	models := model.DefaultModels()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models)
	}
}

func ChatCompletions(cfg ProxyConfig) http.HandlerFunc {
	h := proxy.ProxyHandler(cfg.ChatURL, cfg.BootstrapURL, cfg.Fingerprint)
	return func(w http.ResponseWriter, r *http.Request) {
		h(w, r)
	}
}

func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}