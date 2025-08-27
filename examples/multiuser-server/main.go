package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	godav "github.com/tlmanz/godav"
)

// UploadService coordinates uploads per user with an UploadManager and SSE broadcasting.
type UploadService struct {
	manager *godav.UploadManager

	mu      sync.RWMutex
	clients map[string]*godav.Client // userID -> client

	subsMu sync.RWMutex
	subs   map[string]map[chan string]struct{} // userID -> set of subscriber channels
}

func NewUploadService() *UploadService {
	s := &UploadService{
		manager: godav.NewUploadManager(),
		clients: make(map[string]*godav.Client),
		subs:    make(map[string]map[chan string]struct{}),
	}
	log.Printf("upload service initialized")
	return s
}

// subscribe registers an SSE subscriber for a user and returns a channel and an unsubscribe function.
func (s *UploadService) subscribe(userID string) (chan string, func()) {
	ch := make(chan string, 32)
	s.subsMu.Lock()
	if _, ok := s.subs[userID]; !ok {
		s.subs[userID] = make(map[chan string]struct{})
	}
	s.subs[userID][ch] = struct{}{}
	s.subsMu.Unlock()

	// Log subscribe
	s.subsMu.RLock()
	cnt := len(s.subs[userID])
	s.subsMu.RUnlock()
	log.Printf("sse: user=%s subscribed (subs=%d)", userID, cnt)

	unsub := func() {
		s.subsMu.Lock()
		if m, ok := s.subs[userID]; ok {
			delete(m, ch)
			if len(m) == 0 {
				delete(s.subs, userID)
			}
		}
		s.subsMu.Unlock()
		close(ch)

		// Log unsubscribe
		s.subsMu.RLock()
		cnt := len(s.subs[userID])
		s.subsMu.RUnlock()
		log.Printf("sse: user=%s unsubscribed (subs=%d)", userID, cnt)
	}
	return ch, unsub
}

func (s *UploadService) publish(userID string, payload any, event string) {
	s.subsMu.RLock()
	subs := s.subs[userID]
	s.subsMu.RUnlock()
	if len(subs) == 0 {
		return
	}
	data, _ := json.Marshal(payload)
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, string(data))
	for ch := range subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// getOrCreateClient builds a per-user client and wires config callbacks to publish SSE updates.
func (s *UploadService) getOrCreateClient(userID, baseURL, username, password string) *godav.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.clients[userID]; ok {
		// Reuse existing client
		return c
	}
	c := godav.NewClient(baseURL, username, password)
	cfg := godav.DefaultConfig()
	cfg.ProgressFunc = func(p godav.ProgressInfo) {
		s.publish(userID, p, "progress")
	}
	cfg.EventFunc = func(e godav.EventInfo) {
		s.publish(userID, e, "event")
	}
	c.SetConfig(cfg)
	s.clients[userID] = c
	log.Printf("client: created user=%s baseURL=%s", userID, baseURL)
	return c
}

// StartUpload starts a session for a user and returns the session ID.
func (s *UploadService) StartUpload(userID, baseURL, davUser, davPass, localPath, remotePath string) (string, error) {
	client := s.getOrCreateClient(userID, baseURL, davUser, davPass)
	sess, err := s.manager.AddUploadSession(localPath, remotePath, client)
	if err != nil {
		log.Printf("upload: add-session error user=%s local=%s remote=%s err=%v", userID, localPath, remotePath, err)
		return "", err
	}
	if err := s.manager.StartUpload(sess.ID); err != nil {
		log.Printf("upload: start error user=%s session=%s err=%v", userID, sess.ID, err)
		return "", err
	}
	log.Printf("upload: started user=%s session=%s local=%s remote=%s", userID, sess.ID, localPath, remotePath)
	return sess.ID, nil
}

func (s *UploadService) Pause(id string) error {
	log.Printf("upload: pause session=%s", id)
	return s.manager.PauseUpload(id)
}
func (s *UploadService) Resume(id string) error {
	log.Printf("upload: resume session=%s", id)
	return s.manager.ResumeUpload(id)
}
func (s *UploadService) Cancel(id string) error {
	sess, err := s.manager.GetUploadSession(id)
	if err != nil {
		log.Printf("upload: cancel get-session error session=%s err=%v", id, err)
		return err
	}
	sess.Controller.Cancel()
	log.Printf("upload: cancel signalled session=%s", id)
	return nil
}
func (s *UploadService) Status(id string) (godav.UploadStatus, error) {
	sess, err := s.manager.GetUploadSession(id)
	if err != nil {
		log.Printf("upload: status get-session error session=%s err=%v", id, err)
		return "", err
	}
	log.Printf("upload: status session=%s status=%s", id, sess.Status)
	return sess.Status, nil
}

// HTTP payloads
type startReq struct {
	UserID     string `json:"userId"`
	BaseURL    string `json:"baseURL"`
	DAVUser    string `json:"davUser"`
	DAVPass    string `json:"davPass"`
	LocalPath  string `json:"localPath"`
	RemotePath string `json:"remotePath"`
}
type startResp struct {
	SessionID string `json:"sessionId"`
}
type statusResp struct {
	Status string `json:"status"`
}
type errorResp struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	svc := NewUploadService()
	// Serve a tiny static UI from ./static
	http.Handle("/", http.FileServer(http.Dir("./static")))

	// POST /api/uploads/start
	http.HandleFunc("/api/uploads/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			log.Printf("http: %s %s -> 405", r.Method, r.URL.Path)
			http.Error(w, "method not allowed", 405)
			return
		}
		var req startReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("http: start decode error: %v", err)
			writeJSON(w, 400, errorResp{Error: "invalid json"})
			return
		}
		if req.UserID == "" || req.BaseURL == "" || req.DAVUser == "" || req.DAVPass == "" || req.LocalPath == "" || req.RemotePath == "" {
			log.Printf("http: start missing fields")
			writeJSON(w, 400, errorResp{Error: "missing fields"})
			return
		}
		id, err := svc.StartUpload(req.UserID, req.BaseURL, req.DAVUser, req.DAVPass, req.LocalPath, req.RemotePath)
		if err != nil {
			log.Printf("http: start error user=%s err=%v", req.UserID, err)
			writeJSON(w, 500, errorResp{Error: err.Error()})
			return
		}
		log.Printf("http: start ok user=%s session=%s", req.UserID, id)
		writeJSON(w, 200, startResp{SessionID: id})
	})

	// POST /api/uploads/{id}/pause | /resume
	http.HandleFunc("/api/uploads/", func(w http.ResponseWriter, r *http.Request) {
		// crude routing: /api/uploads/{id}/action
		path := strings.TrimPrefix(r.URL.Path, "/api/uploads/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			log.Printf("http: bad uploads path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		id, action := parts[0], parts[1]
		var err error
		switch action {
		case "pause":
			if r.Method != http.MethodPost {
				log.Printf("http: %s %s -> 405", r.Method, r.URL.Path)
				http.Error(w, "method not allowed", 405)
				return
			}
			err = svc.Pause(id)
		case "resume":
			if r.Method != http.MethodPost {
				log.Printf("http: %s %s -> 405", r.Method, r.URL.Path)
				http.Error(w, "method not allowed", 405)
				return
			}
			err = svc.Resume(id)
		case "status":
			if r.Method != http.MethodGet {
				log.Printf("http: %s %s -> 405", r.Method, r.URL.Path)
				http.Error(w, "method not allowed", 405)
				return
			}
			st, e := svc.Status(id)
			if e != nil {
				log.Printf("http: status error session=%s err=%v", id, e)
				writeJSON(w, 404, errorResp{Error: e.Error()})
				return
			}
			log.Printf("http: status ok session=%s status=%s", id, st)
			writeJSON(w, 200, statusResp{Status: string(st)})
			return
		case "cancel":
			if r.Method != http.MethodDelete {
				log.Printf("http: %s %s -> 405", r.Method, r.URL.Path)
				http.Error(w, "method not allowed", 405)
				return
			}
			err = svc.Cancel(id)
		default:
			log.Printf("http: unknown action %q", action)
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("http: action=%s session=%s error: %v", action, id, err)
			writeJSON(w, 400, errorResp{Error: err.Error()})
			return
		}
		log.Printf("http: action=%s ok session=%s", action, id)
		writeJSON(w, 200, map[string]string{"ok": "true"})
	})

	// GET /api/users/{userId}/stream  (SSE)
	http.HandleFunc("/api/users/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stream") {
			http.NotFound(w, r)
			return
		}
		// path: /api/users/{userId}/stream
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 4 {
			log.Printf("http: bad users path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		userID := parts[2]

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "stream unsupported", 500)
			return
		}

		ch, unsub := svc.subscribe(userID)
		defer unsub()

		// send a hello event
		_, _ = w.Write([]byte("event: hello\ndata: {}\n\n"))
		flusher.Flush()
		log.Printf("sse: connected user=%s", userID)

		keepAlive := time.NewTicker(15 * time.Second)
		defer keepAlive.Stop()

		notify := w.(http.CloseNotifier).CloseNotify()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					log.Printf("sse: channel closed user=%s", userID)
					return
				}
				_, _ = w.Write([]byte(msg))
				flusher.Flush()
			case <-keepAlive.C:
				_, _ = w.Write([]byte(": keep-alive\n\n"))
				flusher.Flush()
			case <-notify:
				log.Printf("sse: client disconnected user=%s", userID)
				return
			}
		}
	})

	addr := ":8080"
	log.Printf("sample server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
