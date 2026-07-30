package db

import (
	"context"
	"strings"
	"testing"
)

func TestRunner_ListCollections(t *testing.T) {
	r := &Runner{}
	_, err := r.ListCollections(context.Background())
	if err == nil {
		t.Fatal("ListCollections() = nil, want error")
	}
	if !strings.Contains(err.Error(), "mongo") {
		t.Fatalf("ListCollections() error = %q, want mongo requirement", err.Error())
	}
}
