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

	var includes, excludes stringSlice
	sizeLimitKB := flag.Int("s", 25, "Max file size in kilobytes (KB).")
	useClaudeXML := flag.Bool("c", false, "Format as Claude XML.")
	useMarkdown := flag.Bool("m", false, "Format as Markdown code blocks.")
	flag.Var(&includes, "i", "Glob pattern to include files (overrides defaults).")
	flag.Var(&excludes, "e", "Glob pattern to exclude files/dirs.")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [directory]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Combines directory contents into a single file for LLMs.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *useClaudeXML && *useMarkdown {
		log.Fatal("Error: Cannot use -c (Claude XML) and -m (Markdown) flags together.")
	}

	rootDir := "."
	if flag.NArg() > 0 {
		rootDir = flag.Arg(0)
	}
	absRootDir, err := filepath.Abs(rootDir)
	if err != nil {
		log.Fatalf("Error getting absolute path for %q: %v", rootDir, err)
	}

	var files []fileData
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
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxSizeBytes || info.Size() == 0 {
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
				return nil // Skip unreadable files quietly
			}
			files = append(files, fileData{path: filepath.ToSlash(relPath), content: content})
		}
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

	if len(files) == 0 {
		log.Println("No files matched. No output generated.")
		return
	}

	var b strings.Builder
	switch {
	case *useClaudeXML:
		buildClaudeXML(&b, files)
	case *useMarkdown:
		buildMarkdown(&b, files)
	default:
		buildStandardText(&b, files)
	}
	fmt.Print(b.String())
}

func getLanguageHint(path string) string {
	return languageExtMap[strings.ToLower(filepath.Ext(path))]
}

func buildMarkdown(b *strings.Builder, files []fileData) {
	for _, file := range files {
		lang := getLanguageHint(file.path)
		b.WriteString(fmt.Sprintf("```%s path=%s\n", lang, file.path))
		b.Write(file.content)
		b.WriteString("\n```\n\n")
	}
}

func buildStandardText(b *strings.Builder, files []fileData) {
	separator := "================================================\n"
	for _, file := range files {
		b.WriteString(separator)
		b.WriteString(fmt.Sprintf("FILE: %s\n", file.path))
		b.WriteString(separator)
		b.Write(file.content)
		b.WriteString("\n\n")
	}
}

func buildClaudeXML(b *strings.Builder, files []fileData) {
	b.WriteString("<documents>\n")
	for _, file := range files {
		b.WriteString("  <document path=\"")
		xml.EscapeText(b, []byte(file.path))
		b.WriteString("\">\n    <content>")
		xml.EscapeText(b, file.content)
		b.WriteString("</content>\n  </document>\n")
	}
	b.WriteString("</documents>\n")
}
