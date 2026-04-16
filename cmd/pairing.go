// cmd/pairing.go — Pairing CLI 子命令。

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/linkxzhou/SimpleClaw/channels"
	"github.com/linkxzhou/SimpleClaw/config"
)

func cmdPairing(args []string) {
	if len(args) == 0 {
		printPairingUsage()
		return
	}

	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	store, err := channels.NewPairingStore(cfg.WorkspacePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load pairing store: %v\n", err)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdPairingList(store)
	case "approve":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: simpleclaw pairing approve <code>")
			os.Exit(1)
		}
		cmdPairingApprove(store, args[1])
	case "revoke":
		cmdPairingRevoke(store, args[1:])
	case "add":
		cmdPairingAdd(store, args[1:])
	case "help", "--help", "-h":
		printPairingUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown pairing command: %s\n", args[0])
		printPairingUsage()
		os.Exit(1)
	}
}

func cmdPairingList(store *channels.PairingStore) {
	pending := store.ListPending()
	approved := store.ListApproved()

	if len(pending) == 0 && len(approved) == 0 {
		fmt.Println("No pairing entries.")
		return
	}

	fmt.Printf("%-12s %-20s %-10s %-10s %s\n", "CHANNEL", "SENDER", "STATUS", "CODE", "REQUESTED AT")
	fmt.Println(strings.Repeat("-", 70))

	for _, e := range approved {
		fmt.Printf("%-12s %-20s %-10s %-10s %s\n",
			e.Channel, e.SenderID, "approved", "-", e.RequestedAt[:10])
	}
	for _, e := range pending {
		fmt.Printf("%-12s %-20s %-10s %-10s %s\n",
			e.Channel, e.SenderID, "pending", e.Code, e.RequestedAt[:10])
	}
}

func cmdPairingApprove(store *channels.PairingStore, code string) {
	entry, err := store.ApproveByCode(code)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Approved: %s:%s\n", entry.Channel, entry.SenderID)
}

func cmdPairingRevoke(store *channels.PairingStore, args []string) {
	var channel, sender string
	for i, a := range args {
		switch a {
		case "--channel":
			if i+1 < len(args) {
				channel = args[i+1]
			}
		case "--sender":
			if i+1 < len(args) {
				sender = args[i+1]
			}
		}
	}
	if channel == "" || sender == "" {
		fmt.Fprintln(os.Stderr, "Usage: simpleclaw pairing revoke --channel <ch> --sender <id>")
		os.Exit(1)
	}
	if err := store.Revoke(channel, sender); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Revoked: %s:%s\n", channel, sender)
}

func cmdPairingAdd(store *channels.PairingStore, args []string) {
	var channel, sender string
	for i, a := range args {
		switch a {
		case "--channel":
			if i+1 < len(args) {
				channel = args[i+1]
			}
		case "--sender":
			if i+1 < len(args) {
				sender = args[i+1]
			}
		}
	}
	if channel == "" || sender == "" {
		fmt.Fprintln(os.Stderr, "Usage: simpleclaw pairing add --channel <ch> --sender <id>")
		os.Exit(1)
	}
	store.AddDirect(channel, sender)
	fmt.Printf("✓ Added: %s:%s\n", channel, sender)
}

func printPairingUsage() {
	fmt.Println(`Pairing — Sender Pairing Authentication

Usage:
  simpleclaw pairing <command>

Commands:
  list                                     列出所有配对状态
  approve <code>                           通过配对码审批
  revoke --channel <ch> --sender <id>      撤销授权
  add --channel <ch> --sender <id>         直接添加授权`)
}
