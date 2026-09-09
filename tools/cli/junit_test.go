package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeReport puts a JUnit report in a temp dir and returns its path.
func writeReport(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.xml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write report: %v", err)
	}
	return path
}

// testcaseAttrs reads a report back and returns the attributes of each
// <testcase>, keyed by the test name.
func testcaseAttrs(t *testing.T, path string) map[string]map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read report: %v", err)
	}
	decoder := xml.NewDecoder(strings.NewReader(string(raw)))
	found := make(map[string]map[string]string)
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "testcase" {
			continue
		}
		attrs := make(map[string]string)
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		found[attrs["name"]] = attrs
	}
	return found
}

func defaultOpts(root string, reportType ReportType) junitOpts {
	return junitOpts{
		root:       root,
		attribute:  "codeowners",
		reportType: reportType,
		inPlace:    true,
	}
}

func TestValidateReportType(t *testing.T) {
	tt := []struct {
		name        string
		input       string
		expected    ReportType
		expectedErr bool
	}{
		{name: "pytest", input: "pytest", expected: TypePytest},
		{name: "jest", input: "jest", expected: TypeJest},
		{name: "auto is no longer accepted", input: "auto", expectedErr: true},
		{name: "unknown framework", input: "mocha", expectedErr: true},
		{name: "empty", input: "", expectedErr: true},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateReportType(tc.input)
			if (err != nil) != tc.expectedErr {
				t.Errorf("validateReportType() error = %v, expectedErr %v", err, tc.expectedErr)
				return
			}
			if got != tc.expected {
				t.Errorf("validateReportType() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestReportTypeDefaults(t *testing.T) {
	tt := []struct {
		reportType   ReportType
		writesFile   bool
		useClassname bool
		exts         []string
	}{
		{reportType: TypePytest, writesFile: true, useClassname: true, exts: []string{".py"}},
		{reportType: TypeJest, writesFile: false, useClassname: false, exts: nil},
	}

	for _, tc := range tt {
		t.Run(string(tc.reportType), func(t *testing.T) {
			if got := tc.reportType.writesFile(); got != tc.writesFile {
				t.Errorf("writesFile() = %v, want %v", got, tc.writesFile)
			}
			if got := tc.reportType.usesClassname(); got != tc.useClassname {
				t.Errorf("usesClassname() = %v, want %v", got, tc.useClassname)
			}
			if got := tc.reportType.extensions(); !slices.Equal(got, tc.exts) {
				t.Errorf("extensions() = %v, want %v", got, tc.exts)
			}
		})
	}
}

func TestAnnotateJUnitJestDoesNotReadClassnameAsPath(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	// "internal.util" is prose here, but it looks exactly like a module path
	// and a real internal/util.go exists. A jest report must not resolve it.
	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="internal.util" name="describe_block_that_looks_like_a_path"/>
<testcase classname="internal.util" name="has_a_real_file" file="frontend/app.js"/>
</testsuite></testsuites>`)

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypeJest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	if got, ok := cases["describe_block_that_looks_like_a_path"]["codeowners"]; ok {
		t.Errorf("classname should not resolve to a path for jest, got %q", got)
	}
	// The file attribute still resolves normally.
	if got := cases["has_a_real_file"]["codeowners"]; got != "@frontend-team" {
		t.Errorf("codeowners = %q, want %q", got, "@frontend-team")
	}
}

func TestAnnotateJUnitByFileAttribute(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="App" name="renders" time="0.1" file="frontend/app.js"/>
<testcase classname="Util" name="helps" time="0.2" file="internal/util.go"/>
</testsuite></testsuites>`)

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypeJest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	if got := cases["renders"]["codeowners"]; got != "@frontend-team" {
		t.Errorf("renders codeowners = %q, want %q", got, "@frontend-team")
	}
	if got := cases["renders"]["codeownersCount"]; got != "1" {
		t.Errorf("renders codeownersCount = %q, want %q", got, "1")
	}
	if got := cases["helps"]["codeowners"]; got != "@backend-team,@security-team" {
		t.Errorf("helps codeowners = %q, want %q", got, "@backend-team,@security-team")
	}
	if got := cases["helps"]["codeownersCount"]; got != "2" {
		t.Errorf("helps codeownersCount = %q, want %q", got, "2")
	}
}

func TestAnnotateJUnitByClassname(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	// A dotted classname is either the module itself or the module plus the
	// test class, and both must resolve to the same file.
	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="internal.util" name="module_level"/>
<testcase classname="internal.util.TestUtil" name="class_level"/>
</testsuite></testsuites>`)

	if err := os.WriteFile(filepath.Join(testRepo, "internal", "util.py"), []byte("# python"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypePytest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	for _, name := range []string{"module_level", "class_level"} {
		if got := cases[name]["codeowners"]; got != "@backend-team,@security-team" {
			t.Errorf("%s codeowners = %q, want %q", name, got, "@backend-team,@security-team")
		}
	}
}

func TestAnnotateJUnitPrefix(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	// The first names its file relative to the prefix, the second relative to
	// the repo root; a mixed report must resolve both.
	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="App" name="prefixed" file="app.ts"/>
<testcase classname="App" name="rooted" file="frontend/app.js"/>
</testsuite></testsuites>`)

	opts := defaultOpts(testRepo, TypeJest)
	opts.prefix = "frontend"
	if err := annotateJUnit([]string{report}, opts); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	for _, name := range []string{"prefixed", "rooted"} {
		if got := cases[name]["codeowners"]; got != "@frontend-team" {
			t.Errorf("%s codeowners = %q, want %q", name, got, "@frontend-team")
		}
	}
}

func TestAnnotateJUnitPytestWritesFile(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="internal.util" name="from_classname"/>
<testcase classname="internal.util.TestUtil" name="from_class"/>
</testsuite></testsuites>`)

	if err := os.WriteFile(filepath.Join(testRepo, "internal", "util.py"), []byte("# python"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypePytest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	for _, name := range []string{"from_classname", "from_class"} {
		if got := cases[name]["file"]; got != "internal/util.py" {
			t.Errorf("%s file = %q, want %q", name, got, "internal/util.py")
		}
	}
}

func TestAnnotateJUnitLeavesUnresolvedTestcasesAlone(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="does.not.exist" name="missing"/>
<testcase classname="Unowned" name="unowned" file="unowned/file.txt"/>
</testsuite></testsuites>`)

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypePytest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	if _, ok := cases["missing"]["codeowners"]; ok {
		t.Error("unresolvable testcase should not be annotated")
	}
	// The file resolves but has no owner, so there is nothing to write.
	if _, ok := cases["unowned"]["codeowners"]; ok {
		t.Error("unowned testcase should not be annotated")
	}
}

func TestAnnotateJUnitPreservesReportContent(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite" tests="1">
<properties><property name="package" value="@scope/pkg"></property></properties>
<testcase classname="App" name="fails" file="frontend/app.js">
<failure message="expected 1 to be 2">stack &lt;trace&gt; here</failure>
</testcase>
</testsuite></testsuites>`)

	if err := annotateJUnit([]string{report}, defaultOpts(testRepo, TypeJest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	raw, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("Failed to read report: %v", err)
	}
	out := string(raw)
	for _, expected := range []string{
		`<property name="package" value="@scope/pkg">`,
		`<failure message="expected 1 to be 2">`,
		`stack &lt;trace&gt; here`,
		`tests="1"`,
		`codeowners="@frontend-team"`,
	} {
		if !strings.Contains(out, expected) {
			t.Errorf("annotated report missing %q\ngot: %s", expected, out)
		}
	}
}

func TestAnnotateJUnitCustomAttribute(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	report := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="suite">
<testcase classname="Util" name="helps" file="internal/util.go"/>
</testsuite></testsuites>`)

	opts := defaultOpts(testRepo, TypeJest)
	opts.attribute = "owners"
	if err := annotateJUnit([]string{report}, opts); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	cases := testcaseAttrs(t, report)
	if got := cases["helps"]["owners"]; got != "@backend-team,@security-team" {
		t.Errorf("owners = %q, want %q", got, "@backend-team,@security-team")
	}
	if got := cases["helps"]["ownersCount"]; got != "2" {
		t.Errorf("ownersCount = %q, want %q", got, "2")
	}
	if _, ok := cases["helps"]["codeowners"]; ok {
		t.Error("default attribute should not be written when overridden")
	}
}

func TestAnnotateJUnitMultipleReportsShareOneLookup(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	first := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="a"><testcase classname="App" name="one" file="frontend/app.js"/></testsuite></testsuites>`)
	second := writeReport(t, `<?xml version="1.0" encoding="utf-8"?>
<testsuites><testsuite name="b"><testcase classname="Util" name="two" file="internal/util.go"/></testsuite></testsuites>`)

	if err := annotateJUnit([]string{first, second}, defaultOpts(testRepo, TypeJest)); err != nil {
		t.Fatalf("annotateJUnit() error = %v", err)
	}

	if got := testcaseAttrs(t, first)["one"]["codeowners"]; got != "@frontend-team" {
		t.Errorf("first report codeowners = %q, want %q", got, "@frontend-team")
	}
	if got := testcaseAttrs(t, second)["two"]["codeowners"]; got != "@backend-team,@security-team" {
		t.Errorf("second report codeowners = %q, want %q", got, "@backend-team,@security-team")
	}
}

func TestAnnotateJUnitErrors(t *testing.T) {
	testRepo, cleanup := setupTestRepo(t)
	defer cleanup()

	tt := []struct {
		name  string
		paths []string
		opts  func(junitOpts) junitOpts
	}{
		{
			name:  "root is not a directory",
			paths: []string{writeReport(t, "<testsuites/>")},
			opts: func(o junitOpts) junitOpts {
				o.root = filepath.Join(testRepo, "main.go")
				return o
			},
		},
		{
			name:  "root is not a git repository",
			paths: []string{writeReport(t, "<testsuites/>")},
			opts: func(o junitOpts) junitOpts {
				o.root = t.TempDir()
				return o
			},
		},
		{
			name:  "report does not exist",
			paths: []string{filepath.Join(testRepo, "no-such-report.xml")},
			opts:  func(o junitOpts) junitOpts { return o },
		},
		{
			name:  "report is not valid xml",
			paths: []string{writeReport(t, "<testsuites><testcase>")},
			opts:  func(o junitOpts) junitOpts { return o },
		},
		{
			name:  "empty attribute name",
			paths: []string{writeReport(t, "<testsuites/>")},
			opts: func(o junitOpts) junitOpts {
				o.attribute = ""
				return o
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			if err := annotateJUnit(tc.paths, tc.opts(defaultOpts(testRepo, TypeJest))); err == nil {
				t.Error("annotateJUnit() expected an error, got nil")
			}
		})
	}
}
