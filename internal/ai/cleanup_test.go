package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/Maimai06/tempo-ai-answer/internal/codemerge"
)

func TestCleanupCaptureBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before ChatSnapshot
		want   int
	}{{"not captured", ChatSnapshot{}, 0}, {"captured empty", ChatSnapshot{Captured: true, IDs: map[string]bool{}}, 2}, {"historical excluded", ChatSnapshot{Captured: true, IDs: map[string]bool{"old": true}}, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			removed := map[string]bool{}
			n, e := CleanupSince(context.Background(), tc.before, func(context.Context) (map[string]bool, error) { return map[string]bool{"old": true, "new": true}, nil }, func(_ context.Context, id string) error { removed[id] = true; return nil })
			if e != nil || n != tc.want || len(removed) != tc.want {
				t.Fatal(n, e, removed)
			}
			if tc.before.IDs["old"] && removed["old"] {
				t.Fatal("deleted historical chat")
			}
		})
	}
	b := ChatSnapshot{Captured: true}
	if _, e := CleanupSince(context.Background(), b, func(context.Context) (map[string]bool, error) { return nil, errors.New("read failure") }, nil); e == nil {
		t.Fatal("read error ignored")
	}
	if _, e := CleanupSince(context.Background(), b, func(context.Context) (map[string]bool, error) { return map[string]bool{"new": true}, nil }, func(context.Context, string) error { return errors.New("delete failure") }); e == nil {
		t.Fatal("delete error ignored")
	}
}

func TestCleanupOwnedDoesNotDeleteUnrelatedNewChats(t *testing.T) {
	owned := map[string]bool{"/a/chat/s/owned": true}
	removed := map[string]bool{}
	n, err := CleanupOwned(context.Background(), owned,
		func(context.Context) (map[string]bool, error) {
			return map[string]bool{
				"/a/chat/s/old":       true,
				"/a/chat/s/owned":     true,
				"/a/chat/s/unrelated": true,
			}, nil
		},
		func(_ context.Context, id string) error { removed[id] = true; return nil },
	)
	if err != nil || n != 1 || len(removed) != 1 || !removed["/a/chat/s/owned"] {
		t.Fatalf("removed = %v, count = %d, error = %v", removed, n, err)
	}
	if removed["/a/chat/s/old"] || removed["/a/chat/s/unrelated"] {
		t.Fatalf("deleted chats not owned by this run: %v", removed)
	}
}

func TestCurrentChatID(t *testing.T) {
	if got := currentChatID("https://chat.example/a/chat/s/session-1?from=home"); got != "/a/chat/s/session-1" {
		t.Fatalf("current chat ID = %q", got)
	}
	if got := currentChatID("https://chat.example/"); got != "" {
		t.Fatalf("home page chat ID = %q", got)
	}
}

func TestDemoFormats(t *testing.T) {
	d := &Demo{}
	ctx := context.Background()
	for _, n := range []int{0, 1, 2} {
		prompt := "【第1题】\n【第2题】\n===FILE: A.java===\n===FILE: B.java==="
		s, e := d.Ask(ctx, prompt, n)
		if e != nil || s == "" {
			t.Fatal(s, e)
		}
		if n > 0 && !codemerge.Complete(s, n) {
			t.Fatal(s)
		}
	}
}
