package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Config holds all CLI configuration options.
type Config struct {
	rootDir      string
	maxDepth     int
	pattern      string
	showTime     bool
	ignoreCase   bool
	extensions   string
	excludeDirs  string
	workers      int
	outputFormat string
	gitRoot      string
	timeout      time.Duration
}

// TodoItem represents a single TODO match in a file.
type TodoItem struct {
	filePath string
	lineNum  int
	content  string
}

// FileTodos groups all TODOs found in a single file.
type FileTodos struct {
	path  string
	todos []TodoItem
}

// gitignoreMatcher holds parsed .gitignore patterns.
type gitignoreMatcher struct {
	patterns []string
	negate   []string
}

// newGitignoreMatcher creates an empty matcher.
func newGitignoreMatcher() *gitignoreMatcher {
	return &gitignoreMatcher{
		patterns: make([]string, 0),
		negate:   make([]string, 0),
	}
}

// addPattern adds a pattern, handling comments and negation (!).
func (g *gitignoreMatcher) addPattern(pattern string) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || strings.HasPrefix(pattern, "#") {
		return
	}
	if strings.HasPrefix(pattern, "!") {
		g.negate = append(g.negate, pattern[1:])
	} else {
		g.patterns = append(g.patterns, pattern)
	}
}

// match checks if a path matches any ignore pattern (respects negation).
func (g *gitignoreMatcher) match(path string) bool {
	for _, neg := range g.negate {
		if matchPattern(neg, path) {
			return false
		}
	}
	for _, pat := range g.patterns {
		if matchPattern(pat, path) {
			return true
		}
	}
	return false
}

// matchPattern matches a single gitignore pattern against a path.
func matchPattern(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "/") {
		pattern = pattern[1:]
		matched, _ := filepath.Match(pattern, filepath.Base(path))
		if matched {
			return true
		}
		return strings.HasPrefix(path, pattern)
	}
	matched, _ := filepath.Match(pattern, filepath.Base(path))
	if matched {
		return true
	}
	if strings.Contains(pattern, "*") {
		regexPattern := "^" + strings.ReplaceAll(pattern, "*", ".*") + "$"
		re := regexp.MustCompile(regexPattern)
		return re.MatchString(path)
	}
	return strings.Contains(path, pattern)
}

// loadGitignore reads .gitignore from rootDir and returns a matcher.
func loadGitignore(rootDir string) *gitignoreMatcher {
	matcher := newGitignoreMatcher()
	gitignorePath := filepath.Join(rootDir, ".gitignore")
	file, err := os.Open(gitignorePath)
	if err != nil {
		return matcher
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		matcher.addPattern(scanner.Text())
	}
	return matcher
}

// shouldSkipDir checks if a directory name is in the exclude list.
func shouldSkipDir(name string, excludeDirs string) bool {
	if excludeDirs == "" {
		return false
	}
	dirs := strings.Split(excludeDirs, ",")
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == name || strings.HasPrefix(name, d+"/") {
			return true
		}
	}
	return false
}

// hasExtension checks if a file has one of the allowed extensions.
func hasExtension(filename, extensions string) bool {
	if extensions == "" {
		return true
	}
	exts := strings.Split(extensions, ",")
	for _, ext := range exts {
		ext = strings.TrimSpace(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

// isBinaryFile checks if a file contains null bytes (likely binary).
func isBinaryFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}
	return false
}

// findTodosInFile scans a file for the regex pattern, returns matching lines.
func findTodosInFile(filePath string, pattern *regexp.Regexp, ignoreCase bool) []TodoItem {
	if isBinaryFile(filePath) {
		return nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var todos []TodoItem
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		matchLine := line
		if ignoreCase {
			matchLine = strings.ToLower(line)
		}
		if pattern.MatchString(matchLine) {
			todos = append(todos, TodoItem{
				filePath: filePath,
				lineNum:  lineNum,
				content:  strings.TrimSpace(line),
			})
		}
	}
	return todos
}

// walkDir traverses the directory tree, sending file paths to the channel.
// Respects gitignore, depth limit, exclude list, and extension filter.
// Cancels on context timeout.
func walkDir(ctx context.Context, rootDir string, matcher *gitignoreMatcher, config *Config, files chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(files)

	filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(rootDir, path)
		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name(), config.excludeDirs) {
				return filepath.SkipDir
			}
			if matcher.match(relPath) {
				return filepath.SkipDir
			}
			depth := strings.Count(relPath, string(os.PathSeparator))
			if config.maxDepth >= 0 && depth >= config.maxDepth {
				return filepath.SkipDir
			}
			return nil
		}

		if matcher.match(relPath) {
			return nil
		}
		if !hasExtension(d.Name(), config.extensions) {
			return nil
		}
		select {
		case files <- path:
		case <-ctx.Done():
		}
		return nil
	})
}

// worker reads file paths from the channel, searches for TODOs, sends results.
// Cancels on context timeout.
func worker(ctx context.Context, id int, files <-chan string, pattern *regexp.Regexp, config *Config, results chan<- FileTodos, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case filePath, ok := <-files:
			if !ok {
				return
			}
			todos := findTodosInFile(filePath, pattern, config.ignoreCase)
			if len(todos) > 0 {
				relPath, _ := filepath.Rel(config.rootDir, filePath)
				select {
				case results <- FileTodos{path: relPath, todos: todos}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// formatTreeOutput builds the tree-style output string from all results.
func formatTree(allTodos []FileTodos, elapsed time.Duration) string {
	if len(allTodos) == 0 {
		return "No TODOs found"
	}

	sort.Slice(allTodos, func(i, j int) bool {
		return allTodos[i].path < allTodos[j].path
	})

	var buf bytes.Buffer
	totalTodos := 0
	totalFiles := len(allTodos)

	for _, ft := range allTodos {
		buf.WriteString(ft.path + "\n")
		for _, todo := range ft.todos {
			totalTodos++
			buf.WriteString(fmt.Sprintf("  L%-4d %s\n", todo.lineNum, todo.content))
		}
		buf.WriteString("\n")
	}

	buf.WriteString(strings.Repeat("─", 24) + "\n")
	buf.WriteString(fmt.Sprintf("%d TODOs • %d files", totalTodos, totalFiles))
	if configShowTime {
		buf.WriteString(fmt.Sprintf(" • %v", elapsed.Round(time.Millisecond)))
	}
	return buf.String()
}

// formatList outputs a flat list: path:line content
func formatList(allTodos []FileTodos, elapsed time.Duration) string {
	if len(allTodos) == 0 {
		return "No TODOs found"
	}

	sort.Slice(allTodos, func(i, j int) bool {
		return allTodos[i].path < allTodos[j].path
	})

	var buf bytes.Buffer
	totalTodos := 0
	totalFiles := len(allTodos)

	for _, ft := range allTodos {
		for _, todo := range ft.todos {
			totalTodos++
			buf.WriteString(fmt.Sprintf("%s:%d %s\n", ft.path, todo.lineNum, todo.content))
		}
	}

	buf.WriteString(strings.Repeat("─", 24) + "\n")
	buf.WriteString(fmt.Sprintf("%d TODOs • %d files", totalTodos, totalFiles))
	if configShowTime {
		buf.WriteString(fmt.Sprintf(" • %v", elapsed.Round(time.Millisecond)))
	}
	return buf.String()
}

// formatJSON outputs results as JSON
func formatJSON(allTodos []FileTodos, elapsed time.Duration) string {
	type jsonTodo struct {
		File string `json:"file"`
		Line int    `json:"line"`
		Text string `json:"text"`
	}
	type jsonOutput struct {
		Todos []jsonTodo `json:"todos"`
		Total int        `json:"total"`
		Files int        `json:"files"`
		Time  string     `json:"time,omitempty"`
	}

	if len(allTodos) == 0 {
		return `{"todos":[],"total":0,"files":0}`
	}

	sort.Slice(allTodos, func(i, j int) bool {
		return allTodos[i].path < allTodos[j].path
	})

	var todos []jsonTodo
	totalTodos := 0
	for _, ft := range allTodos {
		for _, todo := range ft.todos {
			totalTodos++
			todos = append(todos, jsonTodo{
				File: ft.path,
				Line: todo.lineNum,
				Text: todo.content,
			})
		}
	}

	out := jsonOutput{
		Todos: todos,
		Total: totalTodos,
		Files: len(allTodos),
	}
	if configShowTime {
		out.Time = elapsed.Round(time.Millisecond).String()
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	return string(data)
}

var configShowTime bool

func formatOutput(allTodos []FileTodos, config *Config, elapsed time.Duration) string {
	configShowTime = config.showTime
	switch config.outputFormat {
	case "list":
		return formatList(allTodos, elapsed)
	case "json":
		return formatJSON(allTodos, elapsed)
	default:
		return formatTree(allTodos, elapsed)
	}
}

// findGitRoot walks up from startDir to find the nearest .git directory.
func findGitRoot(startDir string) string {
	dir := startDir
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func main() {
	config := Config{}

	flag.StringVar(&config.rootDir, "dir", ".", "Root directory to search")
	flag.IntVar(&config.maxDepth, "depth", -1, "Maximum directory depth (-1 for unlimited)")
	flag.StringVar(&config.pattern, "pattern", "TODO", "Pattern to search for (regex)")
	flag.BoolVar(&config.showTime, "time", false, "Show execution time")
	flag.BoolVar(&config.ignoreCase, "i", false, "Case insensitive search")
	flag.StringVar(&config.extensions, "ext", "", "File extensions to include (comma separated, e.g. .go,.rs)")
	flag.StringVar(&config.excludeDirs, "exclude", ".git,node_modules,vendor,dist,build,target", "Directories to exclude (comma separated)")
	flag.IntVar(&config.workers, "workers", runtime.NumCPU(), "Number of worker goroutines")
	flag.StringVar(&config.outputFormat, "format", "tree", "Output format: tree, list, json")
	flag.StringVar(&config.gitRoot, "git-root", "true", "Search in git repository root (true/false)")
	flag.DurationVar(&config.timeout, "timeout", 2*time.Second, "Timeout for search (e.g. 2s, 500ms)")
	flag.Parse()

	useGitRoot := config.gitRoot == "true" || config.gitRoot == "1" || config.gitRoot == "yes"
	if useGitRoot {
		gitRoot := findGitRoot(config.rootDir)
		if gitRoot != "" {
			config.rootDir = gitRoot
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()

	startTime := time.Now()

	matcher := loadGitignore(config.rootDir)

	patternStr := config.pattern
	if config.ignoreCase {
		patternStr = "(?i)" + patternStr
	}
	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid regex pattern: %v\n", err)
		os.Exit(1)
	}

	files := make(chan string, 1000)
	results := make(chan FileTodos, 1000)

	var wgWalk sync.WaitGroup
	var wgWorkers sync.WaitGroup

	wgWalk.Add(1)
	go walkDir(ctx, config.rootDir, matcher, &config, files, &wgWalk)

	for i := 0; i < config.workers; i++ {
		wgWorkers.Add(1)
		go worker(ctx, i, files, pattern, &config, results, &wgWorkers)
	}

	go func() {
		wgWalk.Wait()
		wgWorkers.Wait()
		close(results)
	}()

	var allTodos []FileTodos
	for ft := range results {
		allTodos = append(allTodos, ft)
	}

	elapsed := time.Since(startTime)
	output := formatOutput(allTodos, &config, elapsed)
	fmt.Println(output)
}