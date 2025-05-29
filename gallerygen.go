package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

//go:embed gallery.tmpl
var templateFS embed.FS

// Version is the current version of the program.
var Version = "<dev>"

var imageExtensions = map[string]bool{
	".gif":  true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

// ImageMetadata contains information about an image file
type ImageMetadata struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ImageGroup represents a group of images that start with the same letter.
type ImageGroup struct {
	Letter   string
	Files    []string
	Metadata map[string]ImageMetadata
}

// Directory represents a directory in the gallery, containing subdirectories and image groups.
type Directory struct {
	Path         string
	Subdirs      []Directory
	ImageGroups  []ImageGroup
	RelativePath string
	Version      string
	Title        string
}

// isHiddenOrTemp returns true if the file should be ignored (hidden or temporary)
func isHiddenOrTemp(filename string) bool {
	base := filepath.Base(filename)
	return strings.HasPrefix(base, ".") || strings.HasSuffix(base, ".tmp") || strings.HasSuffix(base, ".temp")
}

// getImageMetadata returns the dimensions of an image file, using cache if available
func getImageMetadata(imagePath string) (ImageMetadata, error) {
	// Check for cache file
	baseName := filepath.Base(imagePath)
	dirName := filepath.Dir(imagePath)
	cachePath := filepath.Join(dirName, "."+baseName+".gallerygen.json")
	if cacheData, err := os.ReadFile(cachePath); err == nil {
		var metadata ImageMetadata
		if err := json.Unmarshal(cacheData, &metadata); err == nil {
			return metadata, nil
		}
	}

	// If no cache or invalid cache, read the image
	file, err := os.Open(imagePath)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("error opening image file: %v", err)
	}
	defer file.Close()

	img, _, err := image.DecodeConfig(file)
	if err != nil {
		return ImageMetadata{}, fmt.Errorf("error decoding image: %v", err)
	}

	metadata := ImageMetadata{
		Width:  img.Width,
		Height: img.Height,
	}

	// Cache the metadata
	cacheData, err := json.Marshal(metadata)
	if err == nil {
		os.WriteFile(cachePath, cacheData, 0644)
	}

	return metadata, nil
}

func processDirectory(dirPath string, basePath string, title string) (Directory, error) {
	// Calculate relative path from basePath to dirPath
	relPath, err := filepath.Rel(basePath, dirPath)
	if err != nil {
		return Directory{}, fmt.Errorf("error calculating relative path: %v", err)
	}
	if relPath == "." {
		relPath = ""
	}

	dir := Directory{
		Path:         dirPath,
		RelativePath: relPath,
		Version:      Version,
		Title:        title,
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return dir, fmt.Errorf("error reading directory %s: %v", dirPath, err)
	}

	groups := make(map[string][]string)
	metadata := make(map[string]map[string]ImageMetadata)
	var subdirs []Directory

	for _, entry := range entries {
		if entry.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			subdir, err := processDirectory(filepath.Join(dirPath, entry.Name()), basePath, title)
			if err != nil {
				return dir, err
			}
			subdirs = append(subdirs, subdir)
			continue
		}

		// Skip hidden or temporary files
		if isHiddenOrTemp(entry.Name()) {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !imageExtensions[ext] {
			continue
		}

		first := strings.ToUpper(string(entry.Name()[0]))
		groups[first] = append(groups[first], entry.Name())

		// Get image metadata
		imgPath := filepath.Join(dirPath, entry.Name())
		imgMetadata, err := getImageMetadata(imgPath)
		if err != nil {
			log.Printf("Warning: Could not get metadata for %s: %v", imgPath, err)
			continue
		}

		if metadata[first] == nil {
			metadata[first] = make(map[string]ImageMetadata)
		}
		metadata[first][entry.Name()] = imgMetadata
	}

	// Sort group keys
	letters := make([]string, 0, len(groups))
	for k := range groups {
		letters = append(letters, k)
	}
	sort.Strings(letters)

	// Sort files in each group
	imageGroups := make([]ImageGroup, 0, len(letters))
	for _, letter := range letters {
		files := groups[letter]
		sort.Strings(files)
		imageGroups = append(imageGroups, ImageGroup{
			Letter:   letter,
			Files:    files,
			Metadata: metadata[letter],
		})
	}

	dir.ImageGroups = imageGroups
	dir.Subdirs = subdirs
	return dir, nil
}

func generateHTML(dir Directory, tmpl *template.Template) error {
	// Generate index.html for this directory
	outputPath := filepath.Join(dir.Path, "index.html")
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("error creating output file %s: %v", outputPath, err)
	}
	defer f.Close()

	err = tmpl.Execute(f, dir)
	if err != nil {
		return fmt.Errorf("error writing HTML to %s: %v", outputPath, err)
	}

	log.Printf("Generated index.html in directory: %s", dir.Path)

	// Recursively generate index.html for all subdirectories
	for _, subdir := range dir.Subdirs {
		err := generateHTML(subdir, tmpl)
		if err != nil {
			return err
		}
	}

	return nil
}

func watchDirectory(dirPath string, tmpl *template.Template, title string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("error creating watcher: %v", err)
	}
	defer watcher.Close()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Channel to debounce file system events
	debounceChan := make(chan struct{}, 1)
	debounceTimer := time.NewTimer(0)
	<-debounceTimer.C // Drain the initial tick

	// Start watching the directory and all subdirectories
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			if err := watcher.Add(path); err != nil {
				log.Printf("Warning: Could not watch directory %s: %v", path, err)
			} else {
				log.Printf("Now watching directory: %s", path)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error setting up directory watch: %v", err)
	}

	log.Printf("Started watching directory tree at: %s", dirPath)

	for {
		select {
		case event := <-watcher.Events:
			// Ignore events for index.html files and hidden/temp files
			if filepath.Base(event.Name) == "index.html" || isHiddenOrTemp(event.Name) {
				continue
			}

			log.Printf("File system event detected: %s on %s", event.Op, event.Name)

			// Debounce the event
			select {
			case debounceChan <- struct{}{}:
				debounceTimer.Reset(500 * time.Millisecond)
			default:
				// Event already queued
			}

		case err := <-watcher.Errors:
			log.Printf("Watcher error in directory %s: %v", dirPath, err)

		case <-debounceTimer.C:
			<-debounceChan // Clear the debounce channel
			log.Printf("Regenerating gallery for directory: %s", dirPath)

			dir, err := processDirectory(dirPath, dirPath, title)
			if err != nil {
				log.Printf("Error processing directory %s: %v", dirPath, err)
				continue
			}

			if err := generateHTML(dir, tmpl); err != nil {
				log.Printf("Error generating HTML in directory %s: %v", dirPath, err)
			} else {
				log.Printf("Gallery regenerated successfully in directory: %s", dirPath)
			}

		case sig := <-sigChan:
			log.Printf("Received signal %v, shutting down watcher for directory: %s", sig, dirPath)
			return nil
		}
	}
}

func main() {
	dirPath := flag.String("dir", "", "Directory containing images to process. Required.")
	showVersion := flag.Bool("version", false, "Print version and exit.")
	oneshot := flag.Bool("oneshot", false, "Generate gallery once and exit, without watching for changes.")
	title := flag.String("title", "dropbox.dzombak.com/gifs", "Title to use for the gallery.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("gallerygen version %s\n", Version)
		return
	}

	if *dirPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -dir flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"split": strings.Split,
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"until": func(n int) []int {
			var result []int
			for i := 0; i < n; i++ {
				result = append(result, i)
			}
			return result
		},
	}

	templateContent, err := templateFS.ReadFile("gallery.tmpl")
	if err != nil {
		log.Fatalf("Error reading embedded template: %v", err)
	}

	tmpl, err := template.New("gallery.tmpl").Funcs(funcMap).Parse(string(templateContent))
	if err != nil {
		log.Fatalf("Error parsing template: %v", err)
	}

	// Initial generation
	dir, err := processDirectory(*dirPath, *dirPath, *title)
	if err != nil {
		log.Fatalf("Error processing directory %s: %v", *dirPath, err)
	}

	if err := generateHTML(dir, tmpl); err != nil {
		log.Fatalf("Error generating initial HTML in directory %s: %v", *dirPath, err)
	}

	log.Printf("Gallery generation complete in directory: %s", *dirPath)

	// If oneshot mode, exit after initial generation
	if *oneshot {
		return
	}

	// Start watching for changes
	if err := watchDirectory(*dirPath, tmpl, *title); err != nil {
		log.Fatalf("Error watching directory %s: %v", *dirPath, err)
	}
}
