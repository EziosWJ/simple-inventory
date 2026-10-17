// Command mcp-token manages independent credentials through controlled DB
// access. The system administration page is the primary entry point; this
// command stays as a compatible local management path.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	database "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

const usage = `usage: mcp-token issue|list|revoke [flags]

issue  --user-id ID --name NAME [--actor OPERATOR] [--days N | --expires-on YYYY-MM-DD]
       Defaults to a permanent credential; --expires-on is read as Asia/Shanghai.
list   --user-id ID
revoke --id TOKEN_ID --actor OPERATOR
`

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	f := flag.NewFlagSet("mcp-token", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	user := f.Int64("user-id", 0, "exact target user ID")
	name := f.String("name", "", "credential name")
	client := f.String("client", "", "deprecated alias of --name")
	actor := f.String("actor", "", "operator identifier (controlled host identity)")
	scopes := f.String("scopes", "", "deprecated: tools now follow the bound user's permissions")
	days := f.Int("days", 0, "validity in days; 0 means permanent")
	expiresOn := f.String("expires-on", "", "validity end date, Asia/Shanghai, YYYY-MM-DD")
	ttl := f.Duration("ttl", 0, "deprecated alias of --days")
	id := f.Int64("id", 0, "token ID")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	command := args[0]
	if command != "issue" && command != "list" && command != "revoke" {
		return fmt.Errorf("unknown command %q\n%s", command, usage)
	}
	cfg, e := config.Load()
	if e != nil {
		return e
	}
	box, e := mcpserver.NewSecretBox(cfg.JWT.Secret)
	if e != nil {
		return e
	}
	db, e := database.Open(ctx, cfg.Database)
	if e != nil {
		return e
	}
	defer db.Close()
	repo := mcpserver.NewRepository(db.GORM, box)
	if *scopes != "" {
		fmt.Fprintln(os.Stderr, "note: --scopes is deprecated; tool access follows the bound user's current business permissions")
	}
	switch command {
	case "issue":
		label := *name
		if label == "" {
			label = *client
		}
		validity, e := resolveValidity(*days, *expiresOn, *ttl)
		if e != nil {
			return e
		}
		event := audit.Event{Action: "mcp-token.issue", Resource: "mcp-token", Summary: "命令行签发 MCP 令牌"}
		token, raw, e := repo.Create(ctx, mcpserver.CreateOptions{UserID: *user, Name: label, Label: *actor, ExpiresAt: validity}, event)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(map[string]any{"token": raw, "metadata": token})
	case "list":
		page, e := repo.List(ctx, mcpserver.TokenQuery{UserID: *user, Page: 1, PageSize: 500})
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(page.Records)
	case "revoke":
		return repo.Revoke(ctx, *id, *actor)
	}
	return nil
}

// resolveValidity keeps the administration default of a permanent credential
// and converts the deprecated duration alias into whole days.
func resolveValidity(days int, expiresOn string, ttl time.Duration) (*time.Time, error) {
	if days < 0 || days > 3650 {
		return nil, errors.New("--days must be between 0 and 3650")
	}
	if ttl != 0 {
		if days != 0 || expiresOn != "" || ttl < 0 || ttl%(24*time.Hour) != 0 {
			return nil, errors.New("--ttl is deprecated; use --days with a whole number of days")
		}
		days = int(ttl / (24 * time.Hour))
	}
	if days > 0 && expiresOn != "" {
		return nil, errors.New("--days and --expires-on are mutually exclusive")
	}
	if days > 0 {
		expires := time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)
		return &expires, nil
	}
	if strings.TrimSpace(expiresOn) == "" {
		return nil, nil
	}
	return mcpserver.ParseExpiryDate(expiresOn)
}
