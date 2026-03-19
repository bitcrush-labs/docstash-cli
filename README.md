# docstash

CLI for [DocStash](https://docstash.dev), an AI-first document store.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/bitcrush-labs/docstash-cli/main/install.sh | sh
```

Or build from source:

```bash
go install github.com/bitcrush-labs/docstash-cli@latest
```

## Usage

```bash
docstash login                              # Sign in via GitHub or Google
docstash list                               # List your documents
docstash search "kubernetes setup"          # Full-text search
docstash get ID                             # Get document with full content
echo "# Notes" | docstash create --title "Notes" --tags notes
docstash edit ID --old "draft" --new "final"
docstash tags                               # List all tags
```

Run `docstash --help` or `docstash <command> --help` for full usage details.
