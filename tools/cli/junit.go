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
	"unicode"
	"unicode/utf8"

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

const ownerSeparator = ","

const countSuffix = "Count"

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
// as a dotted module path. Trailing class segments in classname are trimmed ("abuse.tests.test_abuse.TestAbuse")
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
	for {
		for _, candidate := range r.candidates(strings.Join(parts, "/")) {
			for _, ext := range r.exts {
				if r.exists(candidate + ext) {
					return candidate + ext
				}
			}
		}
		// Only class segments may be trimmed. Trimming a module segment would
		// walk up into the enclosing package, where an unrelated file of the
		// same name would have its owners stamped onto this test.
		if len(parts) < 2 || !isClassSegment(parts[len(parts)-1]) {
			return ""
		}
		parts = parts[:len(parts)-1]
	}
}

// isClassSegment reports whether a dotted-path segment looks like a test class
// rather than a module. Modules are lower case by convention (PEP 8) and
// pytest only collects classes matching its `python_classes` prefix, which is
// capitalised by default.
func isClassSegment(segment string) bool {
	first, _ := utf8.DecodeRuneInString(segment)
	return unicode.IsUpper(first)
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func removeAttrValue(attrs []xml.Attr, name string) []xml.Attr {
	for i, a := range attrs {
		if a.Name.Local == name {
			return append(attrs[:i], attrs[i+1:]...)
		}
	}
	return attrs
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
func rewrite(raw []byte, files []string, owners map[string][]string, o junitOpts) ([]byte, int, error) {
	out := &bytes.Buffer{}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	encoder := xml.NewEncoder(out)
	i, annotated := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "testcase" {
			// A report may already carry attributes from an earlier run.
			// Clearing them first keeps re-annotation idempotent: a test whose
			// file has since become unowned, or can no longer be resolved at
			// all, must not be left attributed to its former owners.
			start.Attr = removeAttrValue(start.Attr, o.attribute)
			start.Attr = removeAttrValue(start.Attr, o.attribute+countSuffix)

			if file := files[i]; file != "" {
				// The write is only ever additive: a path the framework set
				// itself is left alone, since its consumers may rely on the
				// root it is relative to.
				if o.reportType.writesFile() && attrValue(start.Attr, "file") == "" {
					start.Attr = setAttrValue(start.Attr, "file", file)
				}
				if fileOwners := owners[file]; len(fileOwners) > 0 {
					start.Attr = setAttrValue(start.Attr, o.attribute, strings.Join(fileOwners, ownerSeparator))
					start.Attr = setAttrValue(start.Attr, o.attribute+countSuffix, strconv.Itoa(len(fileOwners)))
					annotated++
				}
			}
			i++
			token = start
		}
		if err := encoder.EncodeToken(token); err != nil {
			return nil, 0, err
		}
	}
	if err := encoder.Close(); err != nil {
		return nil, 0, err
	}
	return out.Bytes(), annotated, nil
}

func annotateJUnit(paths []string, o junitOpts) error {
	if repoStat, err := os.Lstat(o.root); err != nil || !repoStat.IsDir() {
		return fmt.Errorf("root is not a directory: %s", o.root)
	}
	if gitStat, err := os.Stat(filepath.Join(o.root, ".git")); err != nil || !gitStat.IsDir() {
		return fmt.Errorf("root is not a Git repository: %s", o.root)
	}
	if !isXMLName(o.attribute) {
		return fmt.Errorf("attribute is not a valid XML name: %q", o.attribute)
	}

	if !o.inPlace && len(paths) > 1 {
		return fmt.Errorf("writing to stdout supports a single report; use --in-place for %d reports", len(paths))
	}

	// A report may name its files absolutely, and those can only be made
	// repo-relative against an absolute root.
	root, err := filepath.Abs(o.root)
	if err != nil {
		return fmt.Errorf("error resolving root %s: %w", o.root, err)
	}
	o.root = root

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
		// The encoder rejects an XML declaration that is not the first token,
		// so a byte order mark or leading whitespace has to go before the
		// report can be streamed back out.
		raw = bytes.TrimLeft(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), " \t\r\n")
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

	if total > 0 && len(resolved) == 0 {
		return fmt.Errorf("no testcase could be traced back to a file in the repository (%d testcases, all unresolved); check --type and --prefix", total)
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

	annotated := 0
	for _, r := range reports {
		out, count, err := rewrite(r.raw, r.files, fileToOwners, o)
		if err != nil {
			return fmt.Errorf("error rewriting %s: %w", r.path, err)
		}
		annotated += count
		if !o.inPlace {
			fmt.Println(string(out))
			continue
		}
		mode := os.FileMode(0o644)
		if stat, err := os.Stat(r.path); err == nil {
			mode = stat.Mode().Perm()
		}
		if err := os.WriteFile(r.path, out, mode); err != nil {
			return fmt.Errorf("error writing %s: %w", r.path, err)
		}
	}

	_, _ = fmt.Fprintf(os.Stderr, "codeowners: annotated %d of %d testcases (%d resolved, %d unresolved)\n",
		annotated, total, total-unresolved, unresolved)
	return nil
}

func isXMLName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r == '_' || unicode.IsLetter(r):
		case i > 0 && (r == '-' || r == '.' || unicode.IsDigit(r)):
		default:
			return false
		}
	}
	return true
}
