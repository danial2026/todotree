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

### Examples

Search for TODOs in current directory:
```bash
todotree
```

Search for TODO and FIXME, case insensitive:
```bash
todotree -pattern "TODO|FIXME" -i
```

Limit search depth to 2 levels:
```bash
todotree -depth 2
```

Search only Go and Rust files:
```bash
todotree -ext ".go,.rs"
```

Exclude additional directories:
```bash
todotree -exclude ".git,node_modules,dist,tmp"
```

Show execution time:
```bash
todotree -time
```

Search from a different directory:
```bash
todotree -dir /path/to/project
```

Search outside git repository root:
```bash
todotree -git-root=false
```

Set custom timeout (default 2s):
```bash
todotree -timeout 10s
```

## Output Format

```
src/main.rs

  L12   add CLI args
  L41   benchmark parser

src/parser/lexer.rs

  L89   optimize tokenizer

src/tokenizer/mod.rs

  L17   handle UTF-8
  L82   benchmark allocations
  L144  remove unsafe block

tests/parser.rs

  L15   add edge cases

README.md

  L31   document benchmarks

────────────────────────────
7 TODOs • 5 files • 9ms
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