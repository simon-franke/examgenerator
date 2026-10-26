package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/simon-franke/examgenerator/internal/discord"
)

func discordDefault() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("EXAMGEN_DISCORD_ENABLED")), "true")
}
func configuredDiscord() (*discord.Client, error) {
	return discord.New(os.Getenv("EXAMGEN_DISCORD_BOT_TOKEN"), os.Getenv("EXAMGEN_DISCORD_USER_ID"))
}

func runDeliver(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("deliver", flag.ContinueOnError)
	pdf := flags.String("pdf", "", "saved exam PDF to deliver (required)")
	source := flags.String("source", "", "optional LaTeX source to attach")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("deliver: unexpected positional arguments")
	}
	if *pdf == "" {
		return errors.New("deliver: --pdf is required")
	}
	client, err := configuredDiscord()
	if err != nil {
		return err
	}
	paths := []string{*pdf}
	if *source != "" {
		paths = append(paths, *source)
	}
	if err := client.Send(ctx, "Your practice exam is ready. Review the generated questions before use.", paths...); err != nil {
		return fmt.Errorf("Discord delivery failed; files remain local: %w", err)
	}
	fmt.Fprintln(os.Stderr, "delivered exam to your private Discord DM")
	return nil
}
