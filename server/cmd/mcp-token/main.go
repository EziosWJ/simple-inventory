// Command mcp-token manages independent credentials through controlled DB access.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	database "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"io"
	"os"
	"strings"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := run(ctx, os.Args[1:], os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: mcp-token issue|list|revoke --user-id ID --client NAME --actor OPERATOR --scopes tool,... --id TOKEN_ID")
	}
	f := flag.NewFlagSet("mcp-token", flag.ContinueOnError)
	user := f.Int64("user-id", 0, "exact target user ID")
	client := f.String("client", "", "client identifier")
	actor := f.String("actor", "", "operator identifier (controlled host identity)")
	scope := f.String("scopes", "", "comma-separated tool names")
	id := f.Int64("id", 0, "token ID")
	ttl := f.Duration("ttl", 30*24*time.Hour, "validity, maximum 90 days")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	if args[0] != "issue" && args[0] != "list" && args[0] != "revoke" {
		return errors.New("unknown command")
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	db, e := database.Open(ctx, cfg.Database)
	if e != nil {
		return e
	}
	defer db.Close()
	repo := mcpserver.NewRepository(db.GORM)
	switch args[0] {
	case "issue":
		t, raw, e := repo.Issue(ctx, *user, *client, *actor, strings.Split(*scope, ","), *ttl)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]any{"token": raw, "metadata": t})
	case "list":
		v, e := repo.List(ctx, *user)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(v)
	case "revoke":
		return repo.Revoke(ctx, *id, *actor)
	}
	return nil
}
