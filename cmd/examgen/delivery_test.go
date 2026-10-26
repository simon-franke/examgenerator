package main

import (
	"context"
	"testing"
)

func TestDeliverValidation(t *testing.T) {
	for _, args := range [][]string{{}, {"--pdf", "x", "unexpected"}} {
		if err := runDeliver(context.Background(), args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := runDeliver(context.Background(), []string{"-h"}); err != nil {
		t.Fatal(err)
	}
}
func TestDiscordDefaultIsOptIn(t *testing.T) {
	t.Setenv("EXAMGEN_DISCORD_ENABLED", "")
	if discordDefault() {
		t.Fatal("enabled by default")
	}
	t.Setenv("EXAMGEN_DISCORD_ENABLED", "true")
	if !discordDefault() {
		t.Fatal("not enabled")
	}
}
