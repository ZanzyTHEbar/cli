## pangolin login

Login to Pangolin

### Synopsis

Interactive login to select your hosting option and configure access. Use --plain or --json with --no-browser for headless and container environments.

```
pangolin login [hostname] [flags]
```

### Examples

```
  # Container-friendly login without browser or TUI
  pangolin login https://vpn.example.com --plain --no-browser

  # Same flow through the auth subcommand
  pangolin auth login https://vpn.example.com --plain --no-browser

  # Emit the initial device prompt as JSON
  pangolin login https://vpn.example.com --json --no-browser --org-id <org-id>

  # Use PANGOLIN_ENDPOINT in plain/JSON modes
  PANGOLIN_ENDPOINT=https://vpn.example.com pangolin login --plain --no-browser
```

### Options

```
  -h, --help            help for login
      --json            Print the initial device login prompt as JSON
      --no-browser      Do not launch a browser or prompt to open one
      --org-id string   Select an organization by ID without prompting
      --plain           Use line-oriented non-TUI login instructions
```

### SEE ALSO

* [pangolin](pangolin.md)	 - Pangolin CLI

