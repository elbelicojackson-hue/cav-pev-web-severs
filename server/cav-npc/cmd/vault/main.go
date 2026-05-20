// Command vault manages the encrypted API key vault for cav-npc.
//
// Usage:
//
//	vault create --file secrets.vault
//	vault list   --file secrets.vault
//	vault set    --file secrets.vault --key DEEPSEEK_API_KEY --value sk-...
//	vault get    --file secrets.vault --key DEEPSEEK_API_KEY
//
// The master password is read from CAV_NPC_MASTER_PASSWORD env or prompted interactively.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/anthropic-cav/cav-npc/internal/secrets"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	file := fs.String("file", "secrets.vault", "path to vault file")
	key := fs.String("key", "", "key name")
	value := fs.String("value", "", "key value (for set)")
	fs.Parse(os.Args[2:])

	switch cmd {
	case "create":
		doCreate(*file)
	case "list":
		doList(*file)
	case "set":
		doSet(*file, *key, *value)
	case "get":
		doGet(*file, *key)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: vault <command> [flags]

Commands:
  create  Create a new empty vault
  list    List key names in the vault
  set     Add or update a key in the vault
  get     Retrieve a key value from the vault

Flags:
  --file   Path to vault file (default: secrets.vault)
  --key    Key name (for set/get)
  --value  Key value (for set; omit to prompt)

Master password: set CAV_NPC_MASTER_PASSWORD env or enter interactively.
`)
}

func getMasterPassword() string {
	if pw := os.Getenv("CAV_NPC_MASTER_PASSWORD"); pw != "" {
		return pw
	}

	fmt.Fprint(os.Stderr, "Master password: ")
	pw, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		// Fallback for non-terminal (e.g. piped input)
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			return strings.TrimSpace(scanner.Text())
		}
		fmt.Fprintln(os.Stderr, "error: could not read password")
		os.Exit(1)
	}
	return string(pw)
}

func doCreate(file string) {
	if _, err := os.Stat(file); err == nil {
		fmt.Fprintf(os.Stderr, "error: vault file %q already exists\n", file)
		os.Exit(1)
	}

	pw := getMasterPassword()
	fmt.Fprint(os.Stderr, "Confirm password: ")
	pw2, _ := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)

	if pw != string(pw2) {
		fmt.Fprintln(os.Stderr, "error: passwords do not match")
		os.Exit(1)
	}

	if err := secrets.Create(file, pw, map[string]string{}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Vault created: %s\n", file)
}

func doList(file string) {
	pw := getMasterPassword()
	v, err := secrets.Open(file, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	keys := v.Keys()
	if len(keys) == 0 {
		fmt.Println("(empty vault)")
		return
	}
	for _, k := range keys {
		fmt.Println(k)
	}
}

func doSet(file, key, value string) {
	if key == "" {
		fmt.Fprintln(os.Stderr, "error: --key is required")
		os.Exit(1)
	}

	pw := getMasterPassword()

	// Load existing vault (or start fresh if creating)
	var keys map[string]string
	v, err := secrets.Open(file, pw)
	if err != nil {
		if err == secrets.ErrNoVaultFile {
			keys = make(map[string]string)
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Extract all existing keys
		keys = make(map[string]string)
		for _, k := range v.Keys() {
			val, _ := v.Get(k)
			keys[k] = val
		}
	}

	// Prompt for value if not provided
	if value == "" {
		fmt.Fprintf(os.Stderr, "Value for %s: ", key)
		valBytes, _ := term.ReadPassword(int(syscall.Stdin))
		fmt.Fprintln(os.Stderr)
		value = string(valBytes)
	}

	keys[key] = value

	// Re-encrypt and write
	if err := secrets.Create(file, pw, keys); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Key %q set in vault\n", key)
}

func doGet(file, key string) {
	if key == "" {
		fmt.Fprintln(os.Stderr, "error: --key is required")
		os.Exit(1)
	}

	pw := getMasterPassword()
	v, err := secrets.Open(file, pw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	val, err := v.Get(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(val)
}
