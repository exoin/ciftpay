// Command smssink is the local stand-in for Africa's Talking. It accepts the
// same POST /version1/messaging form the real gateway does, stores messages in
// memory and shows them at GET / (HTML) and GET /messages (JSON).
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type message struct {
	ID       string    `json:"id"`
	To       string    `json:"to"`
	From     string    `json:"from"`
	Body     string    `json:"body"`
	Received time.Time `json:"received_at"`
}

type inbox struct {
	mu   sync.Mutex
	msgs []message
	seq  int
}

func (b *inbox) add(to, from, body string) message {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	m := message{ID: fmt.Sprintf("ATXid_%06d", b.seq), To: to, From: from, Body: body, Received: time.Now()}
	b.msgs = append([]message{m}, b.msgs...)
	if len(b.msgs) > 500 {
		b.msgs = b.msgs[:500]
	}
	return m
}

func (b *inbox) list() []message {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]message(nil), b.msgs...)
}

// CSS keeps the US "color" property; the UK spell-checker is told so below.
//
//nolint:misspell // CSS property names are US English
var page = template.Must(template.New("").Parse(`<!doctype html><meta charset="utf-8"><title>CiftPay SMS sink</title>
<style>body{font:15px/1.4 ui-monospace,monospace;background:#F6F1E7;color:#14130F;max-width:720px;margin:2rem auto;padding:0 1rem}
article{border-top:1px dashed #6B675E;padding:.8rem 0}small{color:#6B675E}a{color:#0B3D2E}</style>
<h1>SMS sink</h1><p><small>{{len .}} message(s). POST /version1/messaging to add; GET /messages for JSON.</small></p>
{{range .}}<article><small>{{.Received.Format "15:04:05"}} · {{.ID}} · to {{.To}} from {{.From}}</small><p>{{.Body}}</p></article>{{else}}<p>No messages yet.</p>{{end}}`))

func main() {
	addr := os.Getenv("SMS_SINK_ADDR")
	if addr == "" {
		addr = ":8025"
	}
	box := &inbox{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /version1/messaging", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		to, body := r.Form.Get("to"), r.Form.Get("message")
		if to == "" || body == "" {
			http.Error(w, "to and message are required", http.StatusBadRequest)
			return
		}
		m := box.add(to, r.Form.Get("from"), body)
		log.Printf("sms to=%s id=%s body=%q", to, m.ID, body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"SMSMessageData": map[string]any{
			"Message":    "Sent to 1/1 Total Cost: KES 0.8000",
			"Recipients": []map[string]any{{"number": to, "status": "Success", "statusCode": 101, "messageId": m.ID, "cost": "KES 0.8000"}},
		}})
	})
	mux.HandleFunc("GET /messages", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": box.list()})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, box.list())
	})
	log.Printf("sms-sink listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
