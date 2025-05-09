package main

import (
	"bytes"
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
	var target string

	app := &cli.App{
		Name:        "codeowners-cli",
		Usage:       "CLI tool for working with .codeowners files",
		Description: "",
		Commands: []*cli.Command{
			{
				Name:    "unowned",
				Aliases: []string{"u"},
				Usage:   "Check unowned files in the repository",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.StringFlag{
						Name:        "target",
						Aliases:     []string{"t"},
						Value:       "",
						Usage:       "Path from the root of the repo to the target directory",
						Destination: &target,
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
					return unownedFiles(repo, target, cCtx.Int("depth"), cCtx.Bool("dirs_only"))
				},
			},
			{
				Name:    "owner",
				Aliases: []string{"o"},
				Usage:   "Get owner of a file",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.StringFlag{
						Name:        "target",
						Aliases:     []string{"t"},
						Value:       "",
						Usage:       "Path from the root of the repo to the target file",
						Destination: &target,
					},
				},
				Action: func(cCtx *cli.Context) error {
					return fileOwner(repo, target)
				},
			},
			{
				Name:    "verify",
				Aliases: []string{"v"},
				Usage:   "Verify the .codeowners file",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "root",
						Aliases:     []string{"r", "repo"},
						Value:       "./",
						Usage:       "Path to local Git repo",
						Destination: &repo,
					},
					&cli.StringFlag{
						Name:        "target",
						Aliases:     []string{"t"},
						Value:       "",
						Usage:       "Path from the root of the repo to the target directory with a .codeowners file",
						Destination: &target,
					},
				},
				Action: func(cCtx *cli.Context) error {
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

func fileOwner(repo string, target string) error {
	if repoStat, err := os.Lstat(repo); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", repo)
	}
	if gitStat, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", repo)
	}
	if target == "" {
		return fmt.Errorf("target file is required")
	}
	if targetStat, err := os.Stat(filepath.Join(repo, target)); err != nil || targetStat.IsDir() {
		return fmt.Errorf("target is not a file: %s", target)
	}

	ownersMap, err := codeowners.New(repo, []codeowners.DiffFile{{FileName: target}}, io.Discard)
	if err != nil {
		return fmt.Errorf("error reading codeowners config: %s", err)
	}
	fmt.Println(strings.Join(f.Map(ownersMap.AllRequired(), func(rg *codeowners.ReviewerGroup) string { return rg.ToCommentString() }), "\n"))
	if len(ownersMap.AllOptional()) > 0 {
		fmt.Println("Optional:")
		fmt.Println(strings.Join(f.Map(ownersMap.AllOptional(), func(rg *codeowners.ReviewerGroup) string { return rg.ToCommentString() }), "\n"))
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
