package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/barasher/go-exiftool"
)

// Config holds the application configuration
type Config struct {
	TargetDir string
	DryRun    bool
}

func main() {
	config, err := parseArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	run(config)
}

func parseArgs() (*Config, error) {
	args := os.Args[1:]
	if len(args) < 1 || len(args) > 2 {
		return nil, fmt.Errorf("usage: %s <target_directory> [--dry-run]", os.Args[0])
	}

	config := &Config{
		TargetDir: args[0],
		DryRun:    false,
	}

	if len(args) == 2 {
		if args[1] == "--dry-run" {
			config.DryRun = true
		} else {
			return nil, fmt.Errorf("invalid flag: %s", args[1])
		}
	}
	return config, nil
}

func run(cfg *Config) {
	if _, err := os.Stat(cfg.TargetDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Target directory '%s' not found.\n", cfg.TargetDir)
		os.Exit(1)
	}

	uncategorizedDir := filepath.Join(cfg.TargetDir, "Uncategorized")
	if _, err := os.Stat(uncategorizedDir); os.IsNotExist(err) {
		fmt.Printf("Creating Uncategorized directory: %s\n", uncategorizedDir)
		if !cfg.DryRun {
			if err := os.MkdirAll(uncategorizedDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating Uncategorized directory: %v\n", err)
				os.Exit(1)
			}
		}
	}

	fmt.Println("--- Starting Image Categorization and Renaming ---")
	fmt.Printf("Target Directory: %s\n", cfg.TargetDir)
	if cfg.DryRun {
		fmt.Println("DRY RUN MODE: No files will be moved or renamed.")
	} else {
		fmt.Println("Actual run: Files will be moved and renamed.")
	}
	fmt.Println("-------------------------------------------------")

	entries, err := os.ReadDir(cfg.TargetDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading directory '%s': %v\n", cfg.TargetDir, err)
		os.Exit(1)
	}

	et, err := exiftool.NewExiftool()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing exiftool: %v\n", err)
		os.Exit(1)
	}
	defer et.Close()

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		filePath := filepath.Join(cfg.TargetDir, fileName)

		if strings.ToLower(filepath.Ext(fileName)) != ".jpg" && strings.ToLower(filepath.Ext(fileName)) != ".jpeg" {
			continue
		}

		fmt.Printf("Processing: %s\n", filePath)

		fileInfo := et.ExtractMetadata(filePath)[0]
		if fileInfo.Err != nil {
			fmt.Printf("  Error extracting metadata for %s: %v\n", filePath, fileInfo.Err)
			fmt.Println("  Moving to Uncategorized.")
			if !cfg.DryRun {
				if err := moveFile(filePath, uncategorizedDir, ""); err != nil {
					fmt.Fprintf(os.Stderr, "Error moving file: %v\n", err)
				}
			}
			fmt.Println("-------------------------------------------------")
			continue
		}

		var bestSubject string

		// First, try to get the HierarchicalSubject
		if hierarchicalSubjects, found := fileInfo.Fields["HierarchicalSubject"].([]interface{}); found && len(hierarchicalSubjects) > 0 {
			maxDepth := -1
			for _, subject := range hierarchicalSubjects {
				subjectStr, ok := subject.(string)
				if !ok {
					continue
				}
				currentDepth := strings.Count(subjectStr, "|")
				if currentDepth > maxDepth {
					maxDepth = currentDepth
					bestSubject = subjectStr
				}
			}
			if bestSubject != "" {
				fmt.Printf("  Found HierarchicalSubject: %s\n", bestSubject)
			}
		}

		// If HierarchicalSubject is not found or empty, try to get the regular Subject
		if bestSubject == "" {
			if subject, found := fileInfo.Fields["Subject"].(string); found && subject != "" {
				bestSubject = subject
				fmt.Printf("  No HierarchicalSubject found, falling back to Subject: %s\n", bestSubject)
			}
		}

		if bestSubject != "" {
			parts := strings.Split(bestSubject, "|")

			// First part is the directory name
			categoryDirName := sanitizePathComponent(parts[0])
			destinationDir := filepath.Join(cfg.TargetDir, categoryDirName)

			// Last part is the filename's base
			deepestCategory := sanitizePathComponent(parts[len(parts)-1])

			// Generate a unique hash for the file
			hash, err := fileHash(filePath)
			if err != nil {
				fmt.Printf("  Error generating hash for %s: %v\n", filePath, err)
				fmt.Println("-------------------------------------------------")
				continue
			}

			// Construct the new filename
			newFileName := fmt.Sprintf("%s_%s.jpg", deepestCategory, hash)

			fmt.Printf("  Found top-level category: '%s'\n", categoryDirName)
			fmt.Printf("  Found deepest category: '%s'\n", deepestCategory)
			fmt.Printf("  Destination Directory: '%s'\n", destinationDir)
			fmt.Printf("  New filename will be: '%s'\n", newFileName)

			if !cfg.DryRun {
				if err := os.MkdirAll(destinationDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "Error creating destination directory '%s': %v\n", destinationDir, err)
					// Fallback to moving to Uncategorized if directory creation fails
					fmt.Printf("  Moving '%s' to Uncategorized due to directory creation error.\n", filePath)
					if err := moveFile(filePath, uncategorizedDir, ""); err != nil {
						fmt.Fprintf(os.Stderr, "Error moving file to uncategorized: %v\n", err)
					}
				} else {
					if err := moveFile(filePath, destinationDir, newFileName); err != nil {
						fmt.Fprintf(os.Stderr, "Error moving and renaming file: %v\n", err)
					}
				}
			} else {
				fmt.Printf("  (Dry Run) Would create directory '%s' and rename/move '%s' to '%s'\n", destinationDir, filePath, filepath.Join(destinationDir, newFileName))
			}
		} else {
			// No hierarchical subject or subject found
			fmt.Println("  No Hierarchical Subject or Subject found. Moving to Uncategorized.")
			if !cfg.DryRun {
				if err := moveFile(filePath, uncategorizedDir, ""); err != nil {
					fmt.Fprintf(os.Stderr, "Error moving file: %v\n", err)
				}
			}
		}
		fmt.Println("-------------------------------------------------")
	}

	fmt.Println("--- Renaming and Categorization Complete ---")
}

// moveFile moves a file to the specified destination directory, optionally with a new filename.
func moveFile(srcPath, destDir, newFileName string) error {
	finalDestPath := filepath.Join(destDir, newFileName)
	if newFileName == "" {
		finalDestPath = filepath.Join(destDir, filepath.Base(srcPath))
	}

	err := os.Rename(srcPath, finalDestPath)
	if err != nil {
		return fmt.Errorf("failed to move/rename '%s' to '%s': %w", srcPath, finalDestPath, err)
	}
	fmt.Printf("  Moved '%s' to '%s'\n", srcPath, finalDestPath)
	return nil
}

// fileHash computes a SHA-256 hash of the file's content and returns a truncated version.
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:8], nil // Return a truncated hash for a shorter filename
}

// sanitizePathComponent removes characters that are typically problematic in file or directory names.
// It keeps spaces, hyphens, and underscores.
func sanitizePathComponent(component string) string {
	var result strings.Builder
	for _, r := range component {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('_')
		}
	}
	return result.String()
}
