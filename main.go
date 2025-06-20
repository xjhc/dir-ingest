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
	// defaultExtensions defines files to include by extension (case-insensitive).
	defaultExtensions = map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true, ".java": true, ".c": true, ".h": true, ".cpp": true, ".cs": true, ".rs": true, ".rb": true, ".php": true, ".swift": true, ".kt": true, ".kts": true, ".scala": true, ".pl": true, ".pm": true, ".sh": true,
		".html": true, ".css": true, ".scss": true, ".less": true,
		".json": true, ".yaml": true, ".yml": true, ".xml": true, ".toml": true, ".ini": true, ".env": true,
		".md": true, ".txt": true, ".rst": true, ".sql": true,
	}
	// languageExtMap provides language hints for Markdown formatting based on extension.
	languageExtMap = map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript", ".java": "java", ".c": "c", ".h": "c", ".cpp": "cpp", ".cs": "csharp", ".rs": "rust", ".rb": "ruby", ".php": "php", ".swift": "swift", ".kt": "kotlin", ".kts": "kotlin", ".scala": "scala", ".pl": "perl", ".sh": "bash",
		".html": "html", ".css": "css", ".scss": "scss", ".less": "less",
		".json": "json", ".yaml": "yaml", ".yml": "yaml", ".xml": "xml", ".toml": "toml", ".ini": "ini", ".env": "bash", ".md": "markdown", ".txt": "text", ".rst": "rst", ".sql": "sql",
	}
)

func main() {
	log.SetFlags(0)

	// Use a custom flag set to allow parsing after positional args.
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	var includes, excludes stringSlice
	sizeLimitKB := flagSet.Int("s", 25, "Max file size in kilobytes (KB).")
	useClaudeXML := flagSet.Bool("c", false, "Format as Claude XML.")
	useMarkdown := flagSet.Bool("m", false, "Format as Markdown code blocks.")
	prependPath := flagSet.String("p", "", "Prepend a path to all filenames in the output.")
	flagSet.Var(&includes, "i", "Glob pattern to include files (overrides defaults).")
	flagSet.Var(&excludes, "e", "Glob pattern to exclude files/dirs.")
	flagSet.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Combines directory contents into a single file for LLMs.")
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
		// Handle flags that take a value (e.g., -s 50)
		// This is a heuristic; it assumes a value flag is not a boolean
		// and the next arg doesn't start with a '-'
		if !strings.Contains(arg, "=") {
			flagName := strings.TrimLeft(arg, "-")
			f := flagSet.Lookup(flagName)
			if f != nil {
				// Check if the flag is a boolean type
				isBool := false
				if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
					isBool = b.IsBoolFlag()
				}
				// If not a bool, it expects a value
				if !isBool && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
					flagArgs = append(flagArgs, args[i+1])
					i++ // Consume the value
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

	var files []fileData
	var skippedFiles []string
	maxSizeBytes := int64(*sizeLimitKB) * 1024

	err = filepath.WalkDir(absRootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(absRootDir, path)

		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == ".hg" || name == ".svn" || name == "node_modules" {
				return filepath.SkipDir
			}
			for _, pattern := range excludes {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		for _, pattern := range excludes {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				skippedFiles = append(skippedFiles, fmt.Sprintf("%s (excluded by pattern)", relPath))
				return nil
			}
		}

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

		isIncluded := false
		if len(includes) > 0 {
			for _, pattern := range includes {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					isIncluded = true
					break
				}
			}
		} else {
			isIncluded = defaultExtensions[strings.ToLower(filepath.Ext(relPath))]
		}

		if isIncluded {
			content, err := os.ReadFile(path)
			if err != nil {
				skippedFiles = append(skippedFiles, fmt.Sprintf("%s (unreadable)", relPath))
				return nil
			}
			files = append(files, fileData{path: filepath.ToSlash(relPath), content: content})
		} else {
			if len(includes) == 0 { // Only log if not using custom includes to avoid verbosity
				skippedFiles = append(skippedFiles, fmt.Sprintf("%s (unsupported extension)", relPath))
			}
		}
		return nil
	})

	if err != nil {
		log.Fatalf("Error walking directory %q: %v", rootDir, err)
	}

	if len(skippedFiles) > 0 {
		sort.Strings(skippedFiles)
		log.Printf("Skipped %d files:\n", len(skippedFiles))
		for _, msg := range skippedFiles {
			log.Printf("- %s\n", msg)
		}
		log.Println() // Add a blank line for separation
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