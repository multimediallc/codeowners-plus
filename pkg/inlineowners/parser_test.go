package inlineowners

import (
	"bytes"
	"testing"
)

func TestParse_SingleBlock(t *testing.T) {
	src := `// <CO-inline={@alice, @bob}>
line1
line2
// </CO-inline>
`
	warn := &bytes.Buffer{}
	blocks, err := Parse(src, warn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.StartLine != 2 || b.EndLine != 3 {
		t.Errorf("unexpected line range: got (%d,%d)", b.StartLine, b.EndLine)
	}
	expectedOwners := []string{"@alice", "@bob"}
	if len(b.Owners) != len(expectedOwners) {
		t.Fatalf("expected %d owners, got %d", len(expectedOwners), len(b.Owners))
	}
	for i, o := range expectedOwners {
		if b.Owners[i] != o {
			t.Errorf("owner %d mismatch: expected %s, got %s", i, o, b.Owners[i])
		}
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warnings: %s", warn.String())
	}
}

func TestParse_MultipleBlocksWithHashComments(t *testing.T) {
	src := `# <CO-inline={team1}>
a
# </CO-inline>
# <CO-inline={team2,team3}>
b
c
# </CO-inline>
`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].StartLine != 2 || blocks[0].EndLine != 2 {
		t.Errorf("first block line range wrong: %+v", blocks[0])
	}
	if blocks[1].StartLine != 5 || blocks[1].EndLine != 6 {
		t.Errorf("second block line range wrong: %+v", blocks[1])
	}
}

func TestParse_UnclosedTag(t *testing.T) {
	src := `// <CO-inline={x}>
code
`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if warn.Len() == 0 {
		t.Errorf("expected warning for unclosed tag, got none")
	}
}

func TestParse_NestedTagWarning(t *testing.T) {
	src := `// <CO-inline={a}>
// <CO-inline={b}>
code
// </CO-inline>
// </CO-inline>
`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block (inner ignored), got %d", len(blocks))
	}
	if warn.Len() == 0 {
		t.Errorf("expected warnings for nested tag, got none")
	}
}

func TestParse_SingleLineOpenClose(t *testing.T) {
	src := `   //   <CO-inline={devs}> some code here // </CO-inline>   `
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	blk := blocks[0]
	if blk.StartLine <= blk.EndLine {
		t.Errorf("expected empty content range, got Start=%d End=%d", blk.StartLine, blk.EndLine)
	}
	if len(blk.Owners) != 1 || blk.Owners[0] != "devs" {
		t.Errorf("unexpected owners slice: %+v", blk.Owners)
	}
}

func TestParse_EsotericSpacingAndCase(t *testing.T) {
	src := `#<CO-Inline={team_one ,  team_two}>
# </Co-Inline>
`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	blk := blocks[0]
	if !(len(blk.Owners) == 2 && blk.Owners[0] == "team_one" && blk.Owners[1] == "team_two") {
		t.Errorf("owners parsing failed: %+v", blk.Owners)
	}
}

func TestParse_EmptyOwnerList(t *testing.T) {
	src := `// <CO-inline={}>
// </CO-inline>`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if len(blocks[0].Owners) != 0 {
		t.Errorf("expected empty owner list, got %+v", blocks[0].Owners)
	}
	if warn.Len() == 0 {
		t.Errorf("expected warning for empty owner list, got none")
	}
}

func TestParse_StrayEndTag(t *testing.T) {
	src := `// </CO-inline>`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
	if warn.Len() == 0 {
		t.Errorf("expected warning for stray end tag")
	}
}

func TestParse_AdjacentBlocksNoBlankLine(t *testing.T) {
	src := `// <CO-inline={a}>
// </CO-inline>
// <CO-inline={b}>
code
// </CO-inline>`
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].EndLine >= blocks[1].StartLine {
		t.Errorf("blocks overlap unexpectedly: %+v %+v", blocks[0], blocks[1])
	}
}

func TestParse_WindowsLineEndings(t *testing.T) {
	src := "// <CO-inline={x}>\r\nline\r\n// </CO-inline>\r\n"
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if warn.Len() != 0 {
		t.Errorf("unexpected warning: %s", warn.String())
	}
}

func TestParse_MalformedStartTagIgnored(t *testing.T) {
	src := `// <CO-inline={x}
code` // missing closing }
	warn := &bytes.Buffer{}
	blocks, _ := Parse(src, warn)
	if len(blocks) != 0 {
		t.Fatalf("expected 0 blocks, got %d", len(blocks))
	}
	if warn.Len() == 0 {
		t.Errorf("expected warning for malformed start tag")
	}
}
