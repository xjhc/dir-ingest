# dir-ingest

A minimal Go CLI to combine a directory's source files into a single text block for LLMs.

## Get Started

**1. Install** (Requires [Go](https://go.dev/doc/install) 1.22+)
```bash
go install github.com/xjhc/dir-ingest@latest
```

It uses the [go-gitignore](https://github.com/sabhiram/go-gitignore) library (used in Terraform) to parse `.gitignore` files.

**2. Run**
```bash
# Ingest current dir, using the local .gitignore for exclusions
dir-ingest -m | wl-copy # Linux (Wayland)
dir-ingest -m | xclip -selection clipboard # Linux (X11)
dir-ingest -m | pbcopy # macOS
dir-ingest -m | Set-Clipboard # Windows

# Ingest a specific directory
dir-ingest /path/to/project -m

# Use a different ignore file (like .dockerignore)
dir-ingest -g .dockerignore -m

# Ingest only recognized source code files
dir-ingest -x -m | pbcopy
```

> **Note:** Status messages (like skipped files or the final count) are printed to your terminal's standard error stream. This is intentional, so they appear on your screen for feedback but are **not** copied to your clipboard, which only receives the clean file content.

## Options

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-g` | Path to a `.gitignore`-style file for exclusion rules. | `./.gitignore` |
| `-x` | Only include files with recognized source code extensions. | `false` |
| `-xe` | Extra file extensions to exclude (e.g., `.log`). Can be used multiple times. | |
| `-s` | Max file size in KB. | `25` |
| `-m` | Format as Markdown. | `false` |
| `-c` | Format as Claude XML. | `false` |
| `-p` | Prepend a path to all filenames in the output. | |
| `-v` | Verbose output: displays all files that were copied. | `false` |

Inspiration for this tool comes from [gitingest](https://github.com/cyclotruc/gitingest) by [@cyclotruc](https://github.com/cyclotruc).
