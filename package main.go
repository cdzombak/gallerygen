package main

import (
	"flag"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var imageExtensions = map[string]bool{
	".gif":  true,
	".jpg":  true,
	".jpeg": true,
	".png":  true,
}

type ImageGroup struct {
	Letter string
	Files  []string
}

type Directory struct {
	Path         string
	Subdirs      []Directory
	ImageGroups  []ImageGroup
	RelativePath string
}

func processDirectory(dirPath string, basePath string) (Directory, error) {
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
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return dir, fmt.Errorf("error reading directory %s: %v", dirPath, err)
	}

	groups := make(map[string][]string)
	var subdirs []Directory

	for _, entry := range entries {
		if entry.IsDir() {
			subdir, err := processDirectory(filepath.Join(dirPath, entry.Name()), basePath)
			if err != nil {
				return dir, err
			}
			subdirs = append(subdirs, subdir)
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !imageExtensions[ext] {
			continue
		}
		first := strings.ToUpper(string(entry.Name()[0]))
		groups[first] = append(groups[first], entry.Name())
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
		imageGroups = append(imageGroups, ImageGroup{Letter: letter, Files: files})
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

	// Recursively generate index.html for all subdirectories
	for _, subdir := range dir.Subdirs {
		err := generateHTML(subdir, tmpl)
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	dirPath := flag.String("dir", ".", "Directory containing images to process")
	flag.Parse()

	dir, err := processDirectory(*dirPath, *dirPath)
	if err != nil {
		fmt.Println("Error processing directory:", err)
		return
	}

	// Create template with custom functions
	funcMap := template.FuncMap{
		"split": strings.Split,
		"add": func(a, b int) int {
			return a + b
		},
		"until": func(n int) []int {
			var result []int
			for i := 0; i < n; i++ {
				result = append(result, i)
			}
			return result
		},
	}

	tmpl, err := template.New("gallery.tmpl").Funcs(funcMap).ParseFiles("gallery.tmpl")
	if err != nil {
		fmt.Println("Error loading template:", err)
		return
	}

	err = generateHTML(dir, tmpl)
	if err != nil {
		fmt.Println("Error generating HTML:", err)
		return
	}
}
