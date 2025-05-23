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
		Version:     "v0.3.0.dev",
		Description: "",
		Commands: []*cli.Command{
			{
				Name:        "unowned",
				Aliases:     []string{"u"},
				Usage:       "Check unowned files in the repository",
				UsageText:   "codeowners-cli unowned [options] [target-dir]",
				Description: "Check for unowned files in the repository. If target-dir is specified, only check files under that directory.",
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
				},
				Action: func(cCtx *cli.Context) error {
					target := ""
					if cCtx.NArg() > 0 {
						target = cCtx.Args().First()
					}
					return unownedFiles(repo, target, cCtx.Int("depth"), cCtx.Bool("dirs_only"))
				},
			},
			{
				Name:        "owner",
				Aliases:     []string{"o"},
				Usage:       "Get owner of one or more files",
				UsageText:   "codeowners-cli owner [options] <file1> [file2] [file3]...",
				Description: "Get the owners of one or more files. Multiple files can be specified as arguments.",
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
						Value:   "default",
						Usage:   "Output format.  Allowed values are: default, one-line, and json",
					},
				},
				Action: func(cCtx *cli.Context) error {
					targets := cCtx.Args().Slice()
					if len(targets) == 0 {
						return fmt.Errorf("at least one target file is required")
					}
					allowedFormats := []string{"default", "one-line", "json"}
					format := cCtx.String("format")
					if !slices.Contains(allowedFormats, format) {
						return fmt.Errorf("invalid format %s.  Must be one of %s", format, strings.Join(allowedFormats, ", "))
					}
					return fileOwner(repo, targets, cCtx.String("format"))
				},
			},
			{
				Name:        "verify",
				Aliases:     []string{"v"},
				Usage:       "Verify the .codeowners file",
				UsageText:   "codeowners-cli verify [options] [directory]",
				Description: "Verify the .codeowners file in the specified directory or the root of the repo if not specified. The directory must contain a .codeowners file.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
				},
				Action: func(cCtx *cli.Context) error {
					if cCtx.NArg() == 0 {
						return fmt.Errorf("target directory is required")
					}
					target := cCtx.Args().First()
					return verifyCodeowners(repo, target)
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

func unownedFiles(repo string, target string, depth int, dirsOnly bool) error {
	if repoStat, err := os.Lstat(repo); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", repo)
	}
	if gitStat, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", repo)
	}

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
		if target != "" && !strings.HasPrefix(file, target) {
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
	fmt.Println(strings.Join(unowned, "\n"))
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

func fileOwner(repo string, targets []string, format string) error {
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
	case "json":
		jsonTargets(targets, ownersMap)
	case "default":
		printTargets(targets, ownersMap, false)
	case "one-line":
		printTargets(targets, ownersMap, true)
	}

	return nil
}

func verifyCodeowners(repo string, target string) error {
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
