package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/boyter/gocodewalker"
	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
	"github.com/urfave/cli/v2"
)

func stripRoot(root string, path string) string {
	if root == "." {
		return path
	}
	return strings.TrimPrefix(path, root+"/")
}

func getTargets(cCtx *cli.Context) ([]string, error) {
	var targets []string
	if cCtx.NArg() > 0 {
		targets = cCtx.Args().Slice()
	} else if isStdinPiped() {
		var err error
		targets, err = scanStdin()
		if err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func main() {
	var repo string
	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "Print version",
	}
	cli.VersionPrinter = func(cCtx *cli.Context) {
		fmt.Println(cCtx.App.Version)
	}
	app := &cli.App{
		Name:        "codeowners-cli",
		Usage:       "CLI tool for working with .codeowners files",
		Version:     "v1.3.1",
		Description: "",
		Commands: []*cli.Command{
			{
				Name:        "unowned",
				Aliases:     []string{"u"},
				Usage:       "Check unowned files in the repository",
				UsageText:   "codeowners-cli unowned [options] [target-dir1] [target-dir2]...\n   or: cat dirs.txt | codeowners-cli unowned [options]",
				Description: "Check for unowned files in the repository. Multiple target directories can be specified as arguments or piped from stdin (one directory per line). If no target is specified, checks the entire repository.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.IntFlag{
						Name:    "depth",
						Aliases: []string{"d"},
						Value:   0,
						Usage:   "Directory depth to check (from target)",
					},
					&cli.BoolFlag{
						Name:    "dirs_only",
						Aliases: []string{"do"},
						Value:   false,
						Usage:   "Only check directories",
					},
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   string(FormatDefault),
						Usage:   "Output format. Allowed values are: default, one-line, and json",
					},
				},
				Action: func(cCtx *cli.Context) error {
					targets, err := getTargets(cCtx)
					if err != nil {
						return err
					}

					format, err := validateFormat(cCtx.String("format"))
					if err != nil {
						return err
					}
					return unownedFilesWithFormat(repo, targets, cCtx.Int("depth"), cCtx.Bool("dirs_only"), format)
				},
			},
			{
				Name:        "owner",
				Aliases:     []string{"o"},
				Usage:       "Get owner of one or more files",
				UsageText:   "codeowners-cli owner [options] <file1> [file2] [file3]...\n   or: cat files.txt | codeowners-cli owner [options]",
				Description: "Get the owners of one or more files. Multiple files can be specified as arguments, or piped from stdin (one file per line).",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   string(FormatDefault),
						Usage:   "Output format. Allowed values are: default, one-line, and json",
					},
				},
				Action: func(cCtx *cli.Context) error {
					targets, err := getTargets(cCtx)
					if err != nil {
						return err
					}

					if len(targets) == 0 {
						return fmt.Errorf("no target files provided (either as arguments or from stdin)")
					}

					format, err := validateFormat(cCtx.String("format"))
					if err != nil {
						return err
					}
					return fileOwner(repo, targets, format)
				},
			},
			{
				Name:        "validate",
				Aliases:     []string{"v", "verify"},
				Usage:       "Validate the `.codeowners` file format",
				UsageText:   "codeowners-cli validate [options] <directory1> [directory2]...\n   or: cat dirs.txt | codeowners-cli validate [options]",
				Description: "Validate the `.codeowners` file in the specified directories. Multiple directories can be specified as arguments or piped from stdin (one directory per line). Each directory must contain a `.codeowners` file.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.StringFlag{
						Name:    "format",
						Aliases: []string{"f"},
						Value:   string(FormatDefault),
						Usage:   "Output format. Allowed values are: default, one-line, and json",
					},
				},
				Action: func(cCtx *cli.Context) error {
					targets, err := getTargets(cCtx)
					if err != nil {
						return err
					}

					if len(targets) == 0 {
						fmt.Println("No target provided, validating root .codeowners")
						targets = []string{"."}
					}

					var allErrors []string
					for _, target := range targets {
						if err := validateCodeowners(repo, target); err != nil {
							allErrors = append(allErrors, fmt.Sprintf("%s: %v", target, err))
						}
					}

					if len(allErrors) > 0 {
						return fmt.Errorf("verification failed:\n%s", strings.Join(allErrors, "\n"))
					}
					return nil
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func depthCheck(path string, target string, depth int) bool {
	extra := 0
	if target != "" {
		extra = strings.Count(target, "/") + 1
	}
	return strings.Count(path, "/") > (depth + extra)
}

func unownedFilesWithFormat(repo string, targets []string, depth int, dirsOnly bool, format OutputFormat) error {
	if repoStat, err := os.Lstat(repo); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", repo)
	}
	if gitStat, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", repo)
	}

	// If no targets specified, use empty string to check entire repo
	if len(targets) == 0 {
		targets = []string{""}
	}

	// Process each target
	results := make(map[string][]string)
	for _, target := range targets {
		fileListQueue := make(chan *gocodewalker.File, 100)

		walker := gocodewalker.NewFileWalker(repo, fileListQueue)
		walker.IncludeHidden = true
		walker.ExcludeDirectory = []string{".git"}

		errChan := make(chan error)

		go func() {
			err := walker.Start()
			errChan <- err
			close(errChan)
		}()

		files := make([]codeowners.DiffFile, 0)
		for f := range fileListQueue {
			file := stripRoot(repo, f.Location)
			if depth != 0 && depthCheck(file, target, depth) {
				continue
			}
			if target != "" && !strings.HasPrefix(file, fmt.Sprintf("%s/", target)) {
				continue
			}
			files = append(files, codeowners.DiffFile{FileName: file})
		}

		if err := <-errChan; err != nil {
			return fmt.Errorf("error walking repo: %s", err)
		}

		ownersMap, err := codeowners.New(repo, files, io.Discard)
		if err != nil {
			return fmt.Errorf("error reading codeowners config: %s", err)
		}

		unowned := ownersMap.UnownedFiles()

		if dirsOnly {
			unowned = f.Filtered(f.RemoveDuplicates(f.Map(unowned, func(path string) string {
				return filepath.Dir(path)
			})), func(path string) bool {
				return path != "."
			})
		}
		slices.Sort(unowned)
		if target == "" {
			target = "."
		}
		results[target] = unowned
	}

	// Print results based on format
	switch format {
	case FormatJSON:
		jsonData, err := json.Marshal(results)
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %s", err)
		}
		fmt.Println(string(jsonData))
	case FormatOneLine:
		for target, files := range results {
			if len(targets) > 1 {
				fmt.Printf("%s: ", target)
			}
			fmt.Println(strings.Join(files, ", "))
		}
	default: // FormatDefault
		first := true
		for target, files := range results {
			if !first {
				fmt.Println()
			}
			first = false
			if len(targets) > 1 {
				fmt.Printf("%s:\n", target)
			}
			for _, file := range files {
				fmt.Println(file)
			}
		}
	}

	return nil
}

type Target struct {
	Required []string `json:"required"`
	Optional []string `json:"optional"`
}

func jsonTargets(targets []string, ownersMap codeowners.CodeOwners) {
	requiredOwners := ownersMap.FileRequired()
	optionalOwners := ownersMap.FileOptional()

	targetMap := make(map[string]Target)
	for _, target := range targets {
		targetMap[target] = Target{
			Required: f.Map(requiredOwners[target], func(ro *codeowners.ReviewerGroup) string { return ro.ToCommentString() }),
			Optional: f.Map(optionalOwners[target], func(ro *codeowners.ReviewerGroup) string { return ro.ToCommentString() }),
		}
	}
	jsonString, _ := json.Marshal(targetMap)
	fmt.Println(string(jsonString))
}

func printTargets(targets []string, ownersMap codeowners.CodeOwners, oneLine bool) {
	first := true

	requiredOwners := ownersMap.FileRequired()
	optionalOwners := ownersMap.FileOptional()
	// Process each file
	for _, target := range targets {
		if !first && !oneLine {
			fmt.Println()
		}
		first = false

		if len(targets) > 1 {
			fmt.Printf("%s: ", target)
			if !oneLine {
				fmt.Println()
			}
		}
		printOwners(requiredOwners[target], optionalOwners[target], oneLine)
	}
}

func printOwners(required codeowners.ReviewerGroups, optional codeowners.ReviewerGroups, oneLine bool) {
	sep := "\n"
	if oneLine {
		sep = ", "
	}
	if len(required) > 0 {
		requiredOwnerStrs := f.Map(required, func(rg *codeowners.ReviewerGroup) string {
			return rg.ToCommentString()
		})
		fmt.Print(strings.Join(requiredOwnerStrs, sep))
	}

	if len(required) > 0 && len(optional) > 0 {
		fmt.Print(sep)
	}

	if len(optional) > 0 {
		optionalOwnerStrs := f.Map(optional, func(rg *codeowners.ReviewerGroup) string {
			return rg.ToCommentString() + " (Optional)"
		})
		fmt.Print(strings.Join(optionalOwnerStrs, sep))
	}
	fmt.Println()
}

func fileOwner(repo string, targets []string, format OutputFormat) error {
	if repoStat, err := os.Lstat(repo); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", repo)
	}
	if gitStat, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", repo)
	}

	// Validate all targets exist and are files
	for _, target := range targets {
		if target == "" {
			return fmt.Errorf("empty target file path is not allowed")
		}
		if targetStat, err := os.Stat(filepath.Join(repo, target)); err != nil || targetStat.IsDir() {
			return fmt.Errorf("target is not a file: %s", target)
		}
	}

	// Create diff files for all targets
	diffFiles := make([]codeowners.DiffFile, len(targets))
	for i, target := range targets {
		diffFiles[i] = codeowners.DiffFile{FileName: target}
	}

	ownersMap, err := codeowners.New(repo, diffFiles, io.Discard)
	if err != nil {
		return fmt.Errorf("error reading codeowners config: %s", err)
	}
	switch format {
	case FormatJSON:
		jsonTargets(targets, ownersMap)
	case FormatDefault:
		printTargets(targets, ownersMap, false)
	case FormatOneLine:
		printTargets(targets, ownersMap, true)
	}

	return nil
}

func validateCodeowners(repo string, target string) error {
	if repoStat, err := os.Lstat(repo); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", repo)
	}
	if gitStat, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", repo)
	}
	target = filepath.Join(repo, target)
	if targetStat, err := os.Stat(target); err != nil || !targetStat.IsDir() {
		return fmt.Errorf("target is not a directory: %s", target)
	}
	if ownersStat, err := os.Stat(filepath.Join(target, ".codeowners")); err != nil || ownersStat.IsDir() {
		return fmt.Errorf("target does not contain a .codeowners file: %s", target)
	}
	warningBuffer := bytes.NewBuffer([]byte{})

	rgm := codeowners.NewReviewerGroupMemo()

	codeowners := codeowners.Read(target, rgm, io.Discard)
	if codeowners.Fallback != nil {
		for _, name := range codeowners.Fallback.Names {
			if !strings.HasPrefix(name, "@") {
				_, _ = fmt.Fprintln(warningBuffer, "Fallback owner doesn't start with @: "+name)
			}
		}
	}
	for _, test := range codeowners.OwnerTests {
		for _, name := range test.Reviewer.Names {
			if !strings.HasPrefix(name, "@") {
				_, _ = fmt.Fprintf(warningBuffer, "Owner test (%s) name doesn't start with @: %s\n", test.Match, name)
			}
		}
	}
	for _, test := range codeowners.AdditionalReviewerTests {
		for _, name := range test.Reviewer.Names {
			if !strings.HasPrefix(name, "@") {
				_, _ = fmt.Fprintf(warningBuffer, "Additional reviewer test (%s) name doesn't start with @: %s\n", test.Match, name)
			}
		}
	}
	for _, test := range codeowners.OptionalReviewerTests {
		for _, name := range test.Reviewer.Names {
			if !strings.HasPrefix(name, "@") {
				_, _ = fmt.Fprintf(warningBuffer, "Optional reviewer test (%s) name doesn't start with @: %s\n", test.Match, name)
			}
		}
	}
	if warningBuffer.Len() > 0 {
		return fmt.Errorf("\n%s", strings.ReplaceAll(warningBuffer.String(), "WARNING: ", ""))
	}
	return nil
}
