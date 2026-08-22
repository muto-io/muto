package a2a_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muto-io/muto/core/a2a"
)

func TestNewReturnsErrorOnEmptyURL(t *testing.T) {
	_, err := a2a.New(&a2a.Config{GatewayURL: ""})
	if err == nil {
		t.Fatal("expected error for empty GatewayURL, got nil")
	}
}

func TestSendTaskReturnsTaskID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/send" || r.Method != http.MethodPost {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"taskId": "task-abc",
			"state":  "submitted",
			"output": nil,
		})
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL, AuthToken: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.SendTask(context.Background(), "agent-1", []byte(`{"hello":"world"}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task-abc" {
		t.Errorf("expected task-abc, got %q", result.TaskID)
	}
	if result.State != "submitted" {
		t.Errorf("expected submitted, got %q", result.State)
	}
}

func TestGetTaskStatusReturnsState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tasks/task-xyz/status" || r.Method != http.MethodGet {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"taskId": "task-xyz",
			"state":  "completed",
			"output": []byte(`{"result":"ok"}`),
		})
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.GetTaskStatus(context.Background(), "task-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "completed" {
		t.Errorf("expected completed, got %q", result.State)
	}
}

func TestSendTaskReturnsErrorOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := a2a.New(&a2a.Config{GatewayURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SendTask(context.Background(), "agent-1", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}
