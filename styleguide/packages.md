# Packages

Any package which is vital for use in a CLI tool to parse `.codeowners` files ought to be in `pkg`.  These packages could be easily used by third party developers to build on top of Codeowners Plus.

Packages which require external or environmental dependencies such as `git` and `github` should live in `internal`.  These internal packages should never be imported in `pkg` (exposed) packages.
