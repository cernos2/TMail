// zmail — admin tool for the TMAIL core: domains, mailboxes, aliases, DKIM keys and DNS records.
// Run on the server as: docker compose run --rm ctl <command>
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const usage = `zmail — mail server administration

  migrate                                   apply the database schema
  domain add <domain>                       add a domain
  domain list
  mailbox add <address> [--name "Name"] [--quota 5G] [--password ...]
                                            create a mailbox (without --password one is generated)
  mailbox list [domain]
  mailbox passwd <address> [--password ...] change the password
  mailbox quota <address> <size>            quota: 500M, 5G, 0 = unlimited
  mailbox disable|enable <address>
  alias add <address> <recipient>[,<recipient>...]
  alias list [domain]
  alias del <address>
  user add <login> [--name "Name"] [--password ...] [--admin yes] [--mailboxes a@x,b@x]
                                            webmail user (signs in with the login)
  user list
  user passwd <login> [--password ...]
  user disable|enable <login>
  access grant <login> <mailbox> [--send no] [--personal yes]
                                            give access to a mailbox (default: read and send);
                                            personal mailbox: the user edits its signature
  access revoke <login> <mailbox>
  access list [login]
  dkim gen <domain> [--selector z2026]      create a DKIM key
  dns <domain>                              DNS records to add at the domain registrar
`

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Print(usage)
		return
	}
	ctx := context.Background()
	url := os.Getenv("ZMAIL_DB_URL")
	if url == "" {
		die("ZMAIL_DB_URL is not set")
	}
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		die("database connection: %v", err)
	}
	defer db.Close()
	app := &App{db: db}

	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "migrate":
		err = app.Migrate(ctx)
	case "domain":
		err = app.DomainCmd(ctx, args)
	case "mailbox":
		err = app.MailboxCmd(ctx, args)
	case "alias":
		err = app.AliasCmd(ctx, args)
	case "user":
		err = app.UserCmd(ctx, args)
	case "access":
		err = app.AccessCmd(ctx, args)
	case "dkim":
		err = app.DkimCmd(ctx, args)
	case "dns":
		err = app.DNSCmd(ctx, args)
	default:
		err = fmt.Errorf("unknown command %q\n\n%s", cmd, usage)
	}
	if err != nil {
		die("%v", err)
	}
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", a...)
	os.Exit(1)
}

// splitArgs separates "--flag value" / "--flag=value" pairs from positional arguments,
// so flags may appear anywhere on the command line. All flags take a value.
func splitArgs(args []string) (pos []string, flags map[string]string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if k, v, ok := strings.Cut(name, "="); ok {
			flags[k] = v
			continue
		}
		if i+1 < len(args) {
			flags[name] = args[i+1]
			i++
		} else {
			flags[name] = ""
		}
	}
	return pos, flags
}

func need(pos []string, n int, what string) error {
	if len(pos) < n {
		return fmt.Errorf("missing arguments: %s", what)
	}
	return nil
}
