# todotree

A fast, minimal CLI tool to search for TODOs and other patterns in your codebase, respecting .gitignore rules.

## Features

- Fast parallel file scanning using worker goroutines
- Respects .gitignore patterns (including negation patterns)
- Configurable search depth (like the `tree` command)
- Customizable search patterns with regex support
- Case-insensitive search option
- File extension filtering
- Directory exclusion (default: .git, node_modules, vendor, dist, build, target)
- Tree-style output with file paths and line numbers
- Execution time display
- Automatic git repository root detection
- Configurable timeout to prevent hangs on large directories

## Installation

```bash
go install github.com/danial2026/todotree@latest
```

Or build from source:

```bash
git clone https://github.com/danial2026/todotree
cd todotree
go build -o todotree .
```

## Usage

```bash
todotree [flags]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-dir` | `.` | Root directory to search |
| `-depth` | `-1` | Maximum directory depth (-1 for unlimited) |
| `-git-root` | `true` | Only search in git repository root |
| `-pattern` | `TODO` | Search pattern (regex) |
| `-i` | `false` | Case insensitive search |
| `-ext` | | File extensions to include (comma separated, e.g. `.go,.rs`) |
| `-exclude` | `.git,node_modules,vendor,dist,build,target` | Directories to exclude (comma separated) |
| `-workers` | CPU count | Number of worker goroutines |
| `-timeout` | `2s` | Timeout for search (e.g. `2s`, `500ms`) |
| `-time` | `false` | Show execution time |
| `-format` | `tree` | Output format: tree, list, json |
| `-theme` | `default` | Color theme: default, dracula, monokai, nord, onedark, solarized |

### Examples

Search for TODOs in current directory:
```bash
todotree
```

Search for TODO and FIXME, case insensitive:
```bash
todotree -pattern "TODO|FIXME" -i
```

Search only Go and Rust files:
```bash
todotree -ext ".go,.rs"
```

Exclude additional directories:
```bash
todotree -exclude ".git,node_modules,dist,tmp"
```

> **Note:** The `-git-root` flag accepts multiple formats:
> - `-git-root true` (space-separated)
> - `-git-root=false` (with equals)
> - `-git-root=1` (with equals, numeric)
> - `-git-root yes` (space-separated)
> 
> All other string flags also support space-separated or equals syntax (e.g., `-dir /path`, `-dir=/path`, `-format list`, `-format=list`).

## Output Format

### Tree (default)

```
src/main.rs

  L12   add CLI args
  L41   benchmark parser

src/parser/lexer.rs

  L89   optimize tokenizer

────────────────────────────
7 TODOs • 2 files • 9ms
```

### List (`-format list`)

```
src/main.rs:12 add CLI args
src/main.rs:41 benchmark parser
src/parser/lexer.rs:89 optimize tokenizer
────────────────────────────
7 TODOs • 2 files • 9ms
```

### JSON (`-format json`)

```json
{
  "todos": [
    {"file": "src/main.rs", "line": 12, "text": "add CLI args"},
    {"file": "src/main.rs", "line": 41, "text": "benchmark parser"},
    {"file": "src/parser/lexer.rs", "line": 89, "text": "optimize tokenizer"}
  ],
  "total": 7,
  "files": 2,
  "time": "9ms"
}
```

## How It Works

1. Loads `.gitignore` from the root directory
2. Walks the directory tree in parallel
3. Skips files/directories matching gitignore patterns
4. Filters by depth, extensions, and exclude list
5. Searches each file for the pattern using regex
6. Outputs results in tree format

## Performance

- Uses worker pool for parallel file processing
- Buffered channels for efficient communication
- Minimal memory allocations
- Respects .gitignore to avoid scanning unnecessary files

## License

MIT License - see [LICENSE](LICENSE) for details.
