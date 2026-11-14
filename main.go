package main

import (
	"fmt"
	"os"
	"strings"
)

const version = "0.1.0"

const usage = `devdeck - tiny dev dashboard

Usage:
  devdeck [serve] [--port 7077] [--open]   start the dashboard (127.0.0.1 only)
  devdeck mcp                              run as an MCP server on stdio (for Claude Code)
  devdeck hook <Event>                     Claude Code hook handler (reads JSON on stdin)
  devdeck hooks-config                     print the Claude Code settings snippets to enable devdeck
  devdeck version
`

func main() {
	args := os.Args[1:]
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		runServe(args)
	case "mcp":
		runMCP()
	case "hook":
		runHook(args)
	case "hooks-config":
		printHooksConfig()
	case "version":
		fmt.Println("devdeck", version)
	case "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}
