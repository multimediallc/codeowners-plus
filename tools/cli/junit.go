package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/multimediallc/codeowners-plus/pkg/codeowners"
	f "github.com/multimediallc/codeowners-plus/pkg/functional"
)

// ReportType names the framework that produced a report. Frameworks differ in
// how they identify the file behind a test, so knowing the producer lets the
// right strategy be used and, just as importantly, the wrong one be skipped.
type ReportType string

const (
	// TypePytest reads `classname` as a dotted module path; pytest omits the
	// `file` attribute under its default xunit2 family.
	TypePytest ReportType = "pytest"
	// TypeJest reads the `file` attribute; jest's `classname` holds the text of
	// the describe block, which is prose rather than a path.
	TypeJest ReportType = "jest"
)

var allowedReportTypes = []string{string(TypePytest), string(TypeJest)}

func validateReportType(reportType string) (ReportType, error) {
	if !slices.Contains(allowedReportTypes, reportType) {
		return "", fmt.Errorf("invalid type %s. Must be one of %s", reportType, strings.Join(allowedReportTypes, ", "))
	}
	return ReportType(reportType), nil
}

// writesFile reports whether the resolved path is written back to the `file`
// attribute. Only pytest does: it has no `file` attribute to begin with, so the
// write is purely additive, whereas overwriting one a framework already set
// changes the meaning of a field its consumers may rely on.
func (t ReportType) writesFile() bool {
	return t == TypePytest
}

// extensions returns the file extensions to try when reading `classname` as a
// dotted module path. It is empty for types that never read it that way.
func (t ReportType) extensions() []string {
	if !t.usesClassname() {
		return nil
	}
	return []string{".py"}
}

// usesClassname reports whether `classname` may be read as a dotted module
// path. Doing so for jest would be actively harmful: a describe block named
// something like "chatconnection.reconnectlimiter" looks exactly like a module
// path and could resolve to an unrelated file.
func (t ReportType) usesClassname() bool {
	return t != TypeJest
}

// ownerSeparator joins the owners of a file owned by more than one. A comma is
// unambiguous because a GitHub user or team name cannot contain one.
const ownerSeparator = ","

type junitOpts struct {
	root       string
	prefix     string
	attribute  string
	reportType ReportType
	inPlace    bool
}

// fileResolver maps a <testcase> element back to the repo-relative path of the
// file that defines it, memoizing the filesystem lookups it does along the way.
type fileResolver struct {
	root       string
	prefix     string
	reportType ReportType
	exts       []string
	cache      map[string]bool
}

func newFileResolver(root, prefix string, reportType ReportType) *fileResolver {
	return &fileResolver{
		root:       root,
		prefix:     prefix,
		reportType: reportType,
		exts:       reportType.extensions(),
		cache:      make(map[string]bool),
	}
}

func (r *fileResolver) exists(rel string) bool {
	if rel == "" || strings.HasPrefix(rel, "../") {
		return false
	}
	if found, ok := r.cache[rel]; ok {
		return found
	}
	stat, err := os.Stat(filepath.Join(r.root, rel))
	found := err == nil && !stat.IsDir()
	r.cache[rel] = found
	return found
}

// candidates returns the repo-relative paths to try for a path taken from a
// report, most-specific first. A report may express paths relative to a
// subdirectory (jest names files relative to its own root), so the prefix is
// tried first, then the path as given, which keeps mixed reports working.
func (r *fileResolver) candidates(path string) []string {
	if path == "" {
		return nil
	}
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(r.root, path)
		if err != nil {
			return nil
		}
		return []string{filepath.ToSlash(rel)}
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if r.prefix == "" {
		return []string{clean}
	}
	return []string{filepath.ToSlash(filepath.Join(r.prefix, clean)), clean}
}

// resolve locates the source file for a testcase, using its `file` attribute
// when the framework provides one (jest-junit's addFileAttribute, among
// others) and otherwise, where the report type allows it, reading `classname`
// as a dotted module path.
//
// The dotted form is what pytest emits, where a classname is either the module
// itself ("abuse.tests.test_abuse") or the module plus the test class
// ("abuse.tests.test_abuse.TestAbuse"), so trailing segments are trimmed until
// a real file is found.
func (r *fileResolver) resolve(file, classname string) string {
	for _, candidate := range r.candidates(file) {
		if r.exists(candidate) {
			return candidate
		}
	}

	if classname == "" || !r.reportType.usesClassname() {
		return ""
	}
	parts := strings.Split(classname, ".")
	for len(parts) > 0 {
		for _, candidate := range r.candidates(strings.Join(parts, "/")) {
			for _, ext := range r.exts {
				if r.exists(candidate + ext) {
					return candidate + ext
				}
			}
		}
		parts = parts[:len(parts)-1]
	}
	return ""
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func setAttrValue(attrs []xml.Attr, name, value string) []xml.Attr {
	for i, a := range attrs {
		if a.Name.Local == name {
			attrs[i].Value = value
			return attrs
		}
	}
	return append(attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

// collectTestFiles decodes a report and returns the resolved file for each
// <testcase>, in document order, with "" for any that could not be resolved.
func collectTestFiles(raw []byte, r *fileResolver) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	files := make([]string, 0)
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "testcase" {
			continue
		}
		files = append(files, r.resolve(attrValue(start.Attr, "file"), attrValue(start.Attr, "classname")))
	}
	return files, nil
}

// rewrite streams the report back out, adding ownership attributes to each
// <testcase>. Tokens are copied through untouched, so formatting, comments and
// failure output survive the round trip.
func rewrite(raw []byte, files []string, owners map[string][]string, o junitOpts) ([]byte, error) {
	out := &bytes.Buffer{}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	encoder := xml.NewEncoder(out)
	i := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "testcase" {
			if file := files[i]; file != "" {
				if o.reportType.writesFile() {
					start.Attr = setAttrValue(start.Attr, "file", file)
				}
				if fileOwners := owners[file]; len(fileOwners) > 0 {
					start.Attr = setAttrValue(start.Attr, o.attribute, strings.Join(fileOwners, ownerSeparator))
					start.Attr = setAttrValue(start.Attr, o.attribute+"Count", strconv.Itoa(len(fileOwners)))
				}
			}
			i++
			token = start
		}
		if err := encoder.EncodeToken(token); err != nil {
			return nil, err
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// annotateJUnit writes the owners of each test's source file onto its
// <testcase> element, so that whatever consumes the report downstream can
// group results by ownership.
func annotateJUnit(paths []string, o junitOpts) error {
	if repoStat, err := os.Lstat(o.root); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", o.root)
	}
	if gitStat, err := os.Stat(filepath.Join(o.root, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", o.root)
	}
	if o.attribute == "" {
		return fmt.Errorf("attribute name cannot be empty")
	}

	type report struct {
		path  string
		raw   []byte
		files []string
	}

	resolver := newFileResolver(o.root, o.prefix, o.reportType)
	reports := make([]*report, 0, len(paths))
	resolved := make(map[string]struct{})
	total, unresolved := 0, 0

	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}
		files, err := collectTestFiles(raw, resolver)
		if err != nil {
			return fmt.Errorf("error parsing %s: %w", path, err)
		}
		for _, file := range files {
			total++
			if file == "" {
				unresolved++
				continue
			}
			resolved[file] = struct{}{}
		}
		reports = append(reports, &report{path: path, raw: raw, files: files})
	}

	testFiles := make([]string, 0, len(resolved))
	for file := range resolved {
		testFiles = append(testFiles, file)
	}
	slices.Sort(testFiles)

	diffFiles := f.Map(testFiles, func(file string) codeowners.DiffFile {
		return codeowners.DiffFile{FileName: file}
	})
	ownersMap, err := codeowners.New(o.root, diffFiles, &codeowners.FilesystemReader{}, io.Discard)
	if err != nil {
		return fmt.Errorf("error reading codeowners config: %w", err)
	}
	fileToOwners := mapFilesToOwners(ownersMap)

	for _, r := range reports {
		annotated, err := rewrite(r.raw, r.files, fileToOwners, o)
		if err != nil {
			return fmt.Errorf("error rewriting %s: %w", r.path, err)
		}
		if !o.inPlace {
			fmt.Println(string(annotated))
			continue
		}
		mode := os.FileMode(0o644)
		if stat, err := os.Stat(r.path); err == nil {
			mode = stat.Mode().Perm()
		}
		if err := os.WriteFile(r.path, annotated, mode); err != nil {
			return fmt.Errorf("error writing %s: %w", r.path, err)
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "codeowners: annotated %d of %d testcases (%d unresolved)\n", total-unresolved, total, unresolved)
	return nil
}
