# dir-ingest

A minimal Go CLI to combine a directory's source files into a single text block for LLMs.

It's dependency-free, pipes to your clipboard, and smartly includes common file types by default.

## Get Started

**1. Install** (Requires [Go](https://go.dev/doc/install) 1.22+)
```bash
go install github.com/xjhc/dir-ingest@latest
```
(Ensure `$HOME/go/bin` is in your `PATH`)

**2. Run**
```bash
# Ingest current dir, format as Markdown, and copy to clipboard
dir-ingest -m | wl-copy # Linux (Wayland)
dir-ingest -m | xclip -selection clipboard # Linux (X11)
dir-ingest -m | pbcopy # macOS
dir-ingest -m | Set-Clipboard # Windows

# Ingest a specific directory and put flags after the path
dir-ingest /path/to/project -m

# Prepend a path to all filenames in the output
dir-ingest -p my-project/
```

> **Note:** Status messages (like skipped files or the final count) are printed to your terminal's standard error stream. This is intentional, so they appear on your screen for feedback but are **not** copied to your clipboard, which only receives the clean file content.

## Options

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-m` | Format as Markdown. | `false` |
| `-c` | Format as Claude XML. | `false` |
| `-p` | Prepend a path to all filenames in the output. | |
| `-i` | Glob pattern to include files (overrides defaults). | (see below) |
| `-e` | Glob pattern to exclude files/dirs. | |
| `-s` | Max file size in KB. | `25` |

**Default included extensions:** `.go`, `.py`, `.js`, `.ts`, `.java`, `.c`, `.h`, `.cpp`, `.cs`, `.rs`, `.rb`, `.php`, `.swift`, `.kt`, `.kts`, `.scala`, `.pl`, `.sh`, `.html`, `.css`, `.scss`, `.json`, `.yaml`, `.yml`, `.xml`, `.toml`, `.md`, `.txt`, `.sql`

Inspiration for this tool comes from [gitingest](https://github.com/cyclotruc/gitingest) by [@cyclotruc](https://github.com/cyclotruc).
