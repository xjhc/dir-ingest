package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type fileData struct {
	path    string
	content []byte
}

var (
	// languageExtMap provides language hints for Markdown formatting based on extension.
	languageExtMap = map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript", ".tsx": "tsx", ".java": "java", ".c": "c", ".h": "c", ".cpp": "cpp", ".cs": "csharp", ".rs": "rust", ".rb": "ruby", ".php": "php", ".swift": "swift", ".kt": "kotlin", ".kts": "kotlin", ".scala": "scala", ".pl": "perl", ".sh": "bash",
		".html": "html", ".css": "css", ".scss": "scss", ".less": "less",
		".json": "json", ".yaml": "yaml", ".yml": "yaml", ".xml": "xml", ".toml": "toml", ".ini": "ini", ".md": "markdown", ".txt": "text", ".rst": "rst", ".sql": "sql",
	}
)

func main() {
	log.SetFlags(0)

	// Use a custom flag set to allow parsing after positional args.
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	var excludeExts stringSlice
	gitignorePath := flagSet.String("g", "", "Path to a .gitignore-style file for exclusion rules. Defaults to ./.gitignore if it exists.")
	langOnly := flagSet.Bool("x", false, "Only include files with recognized source code extensions.")
	flagSet.Var(&excludeExts, "xe", "Extra file extensions to exclude (e.g., .log). Can be used multiple times.")
	sizeLimitKB := flagSet.Int("s", 25, "Max file size in kilobytes (KB).")
	useMarkdown := flagSet.Bool("m", false, "Format as Markdown code blocks.")
	useClaudeXML := flagSet.Bool("c", false, "Format as Claude XML.")
	prependPath := flagSet.String("p", "", "Prepend a path to all filenames in the output.")
	verbose := flagSet.Bool("v", false, "Verbose output: displays all files that were copied.")
	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Combines directory contents into a single file for LLMs using .gitignore-style exclusions.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flagSet.PrintDefaults()
	}

	// Manually separate flags and positional arguments
	var flagArgs []string
	var positionalArgs []string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if !strings.Contains(arg, "=") {
			flagName := strings.TrimLeft(arg, "-")
			f := flagSet.Lookup(flagName)
			if f != nil {
				isBool := false
				if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
					isBool = b.IsBoolFlag()
				}
				if !isBool && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					flagArgs = append(flagArgs, args[i+1])
					i++
				}
			}
		}
	}

	flagSet.Parse(flagArgs)

	if *useClaudeXML && *useMarkdown {
		log.Fatal("Error: Cannot use -c (Claude XML) and -m (Markdown) flags together.")
	}

	rootDir := "."
	if len(positionalArgs) > 1 {
		log.Fatal("Error: Only one directory path argument is allowed.")
	}
	if len(positionalArgs) == 1 {
		rootDir = positionalArgs[0]
	}

	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		log.Fatalf("Error getting absolute path for %q: %v", rootDir, err)
	}

	// Initialize gitignore parser
	var ignorer gitignore.IgnoreParser
	// If -g is not provided, default to .gitignore in the root directory.
	if *gitignorePath == "" {
		*gitignorePath = filepath.Join(absRootDir, ".gitignore")
	}
	// Check if the ignore file exists and compile it.
	if _, err := os.Stat(*gitignorePath); err == nil {
		ignorer, err = gitignore.CompileIgnoreFile(*gitignorePath)
		if err != nil {
			log.Fatalf("Error compiling ignore file %q: %v", *gitignorePath, err)
		}
		log.Printf("Using ignore file: %s", *gitignorePath)
	} else {
		log.Println("No .gitignore file found or specified. Proceeding with basic filters.")
	}

	// Prepare a map for fast lookup of excluded extensions.
	excludedExtsMap := make(map[string]bool)
	for _, ext := range excludeExts {
		ext = strings.ToLower(ext)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		excludedExtsMap[ext] = true
	}

	var files []fileData
	var skippedFiles []string
	maxSizeBytes := int64(*sizeLimitKB) * 1024

	err = filepath.WalkDir(absRootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(absRootDir, path)
		if relPath == "." {
			return nil
		}

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".hg" || name == ".svn" || name == "node_modules" {
				return filepath.SkipDir
			}
		}

		// --- Filtering Logic ---
		// 1. Gitignore check
		if ignorer != nil && ignorer.MatchesPath(relPath) {
			reason := "excluded by ignore file"
			// To be more user-friendly, check if it's a directory to give a better message.
			if d.IsDir() {
				skippedFiles = append(skippedFiles, fmt.Sprintf("%s/ (%s)", relPath, reason))
				return filepath.SkipDir
			}
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (%s)", relPath, reason))
			return nil
		}
		if d.IsDir() { // No need to process dirs further
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}
		fileExt := strings.ToLower(filepath.Ext(relPath))

		// 2. Exclusion by extension (-xe)
		if excludedExtsMap[fileExt] {
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (extension excluded by -xe)", relPath))
			return nil
		}

		// 3. Language-only filter (-x)
		if *langOnly {
			if _, isLang := languageExtMap[fileExt]; !isLang {
				skippedFiles = append(skippedFiles, fmt.Sprintf("%s (not a recognized source file with -x)", relPath))
				return nil
			}
		}

		// 4. Size and empty file checks.
		info, err := d.Info()
		if err != nil {
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (error getting info)", relPath))
			return nil
		}
		if info.Size() > maxSizeBytes {
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (too large: %dKB > %dKB)", relPath, info.Size()/1024, *sizeLimitKB))
			return nil
		}
		if info.Size() == 0 {
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (empty)", relPath))
			return nil
		}

		// All checks passed, read the file.
		content, err := os.ReadFile(path)
		if err != nil {
			skippedFiles = append(skippedFiles, fmt.Sprintf("%s (unreadable)", relPath))
			return nil
		}
		files = append(files, fileData{path: filepath.ToSlash(relPath), content: content})

		return nil
	})

	if err != nil {
		log.Fatalf("Error walking directory %q: %v", rootDir, err)
	}

	// Sort files to ensure README.md comes first, then by path.
	sort.Slice(files, func(i, j int) bool {
		isReadmeI := strings.EqualFold(filepath.Base(files[i].path), "readme.md")
		isReadmeJ := strings.EqualFold(filepath.Base(files[j].path), "readme.md")
		if isReadmeI != isReadmeJ {
			return isReadmeI
		}
		return files[i].path < files[j].path
	})

	// --- Reporting ---
	if len(skippedFiles) > 0 {
		sort.Strings(skippedFiles)
		log.Printf("Skipped %d files/dirs:\n", len(skippedFiles))
		for _, msg := range skippedFiles {
			log.Printf("- %s\n", msg)
		}
		log.Println() // Add a blank line for separation
	}

	if *verbose && len(files) > 0 {
		log.Printf("Copied %d files:\n", len(files))
		for _, file := range files {
			log.Printf("- %s\n", file.path)
		}
		log.Println() // Add a blank line for separation
	}

	// --- Output Generation ---
	if len(files) == 0 {
		log.Println("No files matched. No output generated.")
		return
	}

	var b strings.Builder
	switch {
	case *useClaudeXML:
		buildClaudeXML(&b, files, *prependPath)
	case *useMarkdown:
		buildMarkdown(&b, files, *prependPath)
	default:
		buildStandardText(&b, files, *prependPath)
	}
	fmt.Print(b.String())
	log.Printf("✓ Ingested %d files.", len(files))
}

func getLanguageHint(path string) string {
	return languageExtMap[strings.ToLower(filepath.Ext(path))]
}

func applyPrepend(path, prepend string) string {
	if prepend == "" {
		return path
	}
	return filepath.ToSlash(filepath.Join(prepend, path))
}

func buildMarkdown(b *strings.Builder, files []fileData, prependPath string) {
	for _, file := range files {
		outputPath := applyPrepend(file.path, prependPath)
		lang := getLanguageHint(file.path)
		b.WriteString(fmt.Sprintf("```%s path=%s\n", lang, outputPath))
		b.Write(file.content)
		// Ensure content ends with a newline before the closing fence
		if len(file.content) > 0 && file.content[len(file.content)-1] != '\n' {
			b.WriteString("\n")
		}
		b.WriteString("```\n\n")
	}
}

func buildStandardText(b *strings.Builder, files []fileData, prependPath string) {
	separator := "================================================\n"
	for _, file := range files {
		outputPath := applyPrepend(file.path, prependPath)
		b.WriteString(separator)
		b.WriteString(fmt.Sprintf("FILE: %s\n", outputPath))
		b.WriteString(separator)
		b.Write(file.content)
		b.WriteString("\n\n")
	}
}

func buildClaudeXML(b *strings.Builder, files []fileData, prependPath string) {
	b.WriteString("<documents>\n")
	for _, file := range files {
		outputPath := applyPrepend(file.path, prependPath)
		b.WriteString("  <document path=\"")
		xml.EscapeText(b, []byte(outputPath))
		b.WriteString("\">\n    <content>")
		xml.EscapeText(b, file.content)
		b.WriteString("</content>\n  </document>\n")
	}
	b.WriteString("</documents>\n")
}
