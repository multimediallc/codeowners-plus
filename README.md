# Codeowners Plus

Code Ownership &amp; Review Assignment Tool - GitHub CODEOWNERS but better

[![Go Report Card](https://goreportcard.com/badge/github.com/multimediallc/codeowners-plus)](https://goreportcard.com/report/github.com/multimediallc/codeowners-plus?kill_cache=1)
[![Tests](https://github.com/multimediallc/codeowners-plus/actions/workflows/go.yml/badge.svg)](https://github.com/multimediallc/codeowners-plus/actions/workflows/go.yml)
![Coverage](https://img.shields.io/badge/Coverage-83.4%25-brightgreen)
[![License](https://img.shields.io/badge/License-BSD%203--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)
[![Contributor Covenant](https://img.shields.io/badge/Contributor%20Covenant-2.1-4baaaa.svg)](CODE_OF_CONDUCT.md)

This tool is meant to be an alternative to [GitHub CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners), overcoming limitations with the `CODEOWNERS` spec and GitHub code owners feature.

## Features

These are features missing from GitHub code owners that are supported by Codeowners Plus

* Smart dismissal of stale reviews (only dismiss review when owned files change, rather than any reviewable push)
* Supports multiple owners of files (`AND` ownership rules)
  * Github CODEOWNERS supports only `OR` ownership rules, in contrast
* Directory-level code ownership files to assign fine-grained code ownership
* Supports optional reviewers (cc users/teams for non-blocking reviews)
* Advanced global configuration (see [Advanced Configuration](#advanced-configuration))

### Caveats & Disclaimer

We hope that GitHub integrates some of these ideas into the code owners feature itself.
Unfortunately, because code owners does not involve "AI", GitHub will likely not prioritize it.

Because this tool is not built into GitHub itself, it does not get first-party integration like the owners badge next to the reveiewer's name.
Instead, this tool uses comments to communicate who the required reviews are, and uses status check as the default way to enforce required reviews.

## Getting Started

### Create a `.codeowners` file in the root of your repository with the following contents:

```
# fallback owner for any file
* @your-username
```

This rule will set up your repo so `@your-username` will become a required approval for any file changed.  When you are ready to add more rules, see [.codeowners File Spec](#codeowners-file-spec)

### Github Protections Configuration

The `Codeowners Plus` GitHub Action should be set up as a [required status check](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches#require-status-checks-before-merging) in a Github Workflow.  The workflow should be run on `pull_request`.  It is recommended to also set up a rerun workflow on `pull_request_review` to rerun the check (see [.github/workflows/rerun_codeowners.yml](.github/workflows/rerun_codeowners.yml) for an example).

**For advanced features to work, such as only re-requesting review when owned files are changed, you must disable this rule in branch protections:**
`Dismiss stale pull request approvals when new commits are pushed`

### Example workflow

```
name: 'Code Owners'

concurrency:
  group: codeowners-${{ github.ref }}
  cancel-in-progress: true

on:
  pull_request:
    types: [opened, reopened, synchronize, ready_for_review, labeled, unlabeled]

jobs:
  codeowners:
    name: 'Run Codeowners Plus'
    runs-on: ubuntu-latest
    if: ${{ !github.event.pull_request.draft }}
    steps:
      - name: 'Checkout Code Repository'
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: 'Codeowners Plus'
        uses: multimediallc/codeowners-plus@v0.1.4
        with:
          github-token: '${{ secrets.GITHUB_TOKEN }}'
          pr: '${{ github.event.pull_request.number }}'
```

### Github Teams support

If you plan to have organization teams as code owners, you will need to use a PAT that has organization [read access for Members and Administration](https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens) as the token.  If do not have organization teams as owners, [GITHUB_TOKEN](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication#using-the-github_token-in-a-workflow) should be sufficient.

## .codeowners File Spec

It is recommended to have a `.codeowners` file in each directory.  However, if there is no `.codeowners` file in a directory, Codeowners Plus will still recursively check rules from higher in the directory tree.

To set fallback (default) owners for any file in the directory, use `*`:
```
* @your-org/dir-owner-team
```

To set owners for a specific file in the given directory, use the file name:
```
# matches only one file in this directory with the name `file_name.md`
file_name.md @your-org/file-owner-team
```

To set wildcard rules, use globstar pattern matching:
```
# matches all files in this directory starts with `test` and ends in `.py` - for example `test_one.py`
test*.py @your-org/tests-owner-team
# matches all files in this directory that include `config` in the filename
*config*
# matches all files in this directory or any subdirectory ending in `.js`
**/*.js
# matches all files in any directory called models
**/models/**
# matches any file in a subdirectory that starts with `prefix_`
prefix_*/**

# Note - `**` will be treated like `*` if it is not surrounded by path separators - for example
test**.txt
# will be treated the same as
test*.txt
```
For more details, see [doublestar patterns](https://github.com/bmatcuk/doublestar#patterns).

By default, each file will resolve to a single reviewer.  See [priority section](#priority) for details about how the owner is assigned.

To instead require an owner as an additional reviewer (`AND` rule), put an `&` at the start of the line:
```
# this rule will add `@task-auditor` as a required review in addition to the file owner's required review
& **/task.go @task-auditor
```

You can also add non-owner (view-only / optional) reviewers by putting a `?` at the start of the line:
```
# this rule will cause `@your-org/data-team` to be alerted about the pull request without requiring them to submit a review
? **/migrations/** @your-org/data-team
```
Non-owner reviewers will be pinged in a comment, but not added as reviewers

Any rule can also be an `OR` rule by putting 2 or more teams on the same line:
```
& **/models.py @models-owner-1 @models-owner-2
```
In the above rule, either `@models-owner-1` OR `models-owner-2` can dismiss this additional reviewer rule for `models.py` files

Comments are also suppored:
```
# This is a comment
```

### Matching differences from Github CODEOWNERS

`.codeowners` rules are always relative to the directory the file lives in, while `CODEOWNERS` are not relative.
For example - `*.js` in `CODEOWNERS` matches any file in the repo that ends in `.js` but in `.codeowners` it will only match files in its directory that end in `.js`.
To match any file that ends in `.js`, use `**/*.js`

Because all rules in a `.codeowners` file are relative to its directory, leading `/` are ignored.  By contrast, `CODEOWNERS` uses leading `/` to target rules from the root of the repository.

`.codeowners` rules do not directly support the `CODEOWNERS` trailing `/` to match any file in a directory (example `apps/` to match any file in an apps directory).
However, the parser will replace trailing `/` with trailing `/**` to indirectly support it.

### Priority

This is the priority of file owners in order from highest priority to lowest priority:

* specific file owner
* wildcard file owner
* globstar file owner
* globstar file owner from parent dirs (recursive)
* fallback owner for dir
* fallback owner from parent dir (recursive)

This means if there is overlap in rules, the last declared will be the owner.  This is similar to GitHub `CODEOWNERS`, except with type-of-rule priority.

## Advanced Configuration

You can include a `codeowners.toml` in the root of your project to add some avanced configuration options.

### Enforcement Options

In some cases, it may be undesirable to fail the status check - for example if you want to keep status check badge reflecting the health of PR code.
If you want to prevent Codeowners Plus check from failing, but instead add an approval to the PR, you can use `enforcement.approval` setting.
If required reviews are satisfied, Codeowners Plus will trigger an approval.  Otherwise, Codeowners Plus will dismiss its prior approval.

Notably, this can be used in conjuction with `CODEOWNERS` if the token owner is a user.
Unfortunately, `CODEOWNERS` does not support apps/bots as owners despite there being an [active discussion requesting the feature since 2020](https://github.com/orgs/community/discussions/23064).
Hopefully GitHub adds support for apps/bots as codeowners so this option can become viable for non-org repos.

Example of using `enforcement.approval` setting for enforcement instead of status check:

`codeowners.toml`
```toml
# `enforcement` allows you to specify how the Codeowners Plus check should be enforced
[enforcement]
# `approval` (default false) means the github token owner will approve the PR
# this can be used alongside GitHub CODEOWNERS to enforce Codeowners Plus checks by making the github token owner the only CODEOWNER
approval = true
# `fail_check` (default true) means the codeowners GHA check will fail if the codeowners check fails
fail_check = false
```

`.github/CODEOWNERS`
```
# NOTE: This should be the only rule
# github token owner (GitHub user set up to act as a bot)
* @token-owner
```

With this setup, `@token-owner` will be a GitHub code owner on every PR, and Codeowners Plus will approve on behalf of `@token-owner` when all required reviews are satisfied, unblocking merge.

### Min Reviews

If you want to use Codeowners Plus to enforce minimum number of reviewers, you can use `min_reviews` setting:

`codeowners.toml`
```toml
# `min_reviews` (default nil) allows you to specify the minimum number of reviews required
min_reviews = 1
```

### Max Reviews

If you want to allow PR authors to bypass some reviews when there are a large numeber of owners involved, you can use the `max_reviews` setting:

`codeowners.toml`
```toml
#
# `max_reviews` (default nil) allows you to skip some reviewers if the number of reviewers is greater than the max_reviewers
max_reviews = 2
```

You can prevent certain owners from being bypassed by the max_reviews setting with the `unskippable_reviewers` setting:

`codeowners.toml`
```toml
# `unskippable_reviewers` (default empty) allows you to specify reviewers that cannot be skipped via the max_reviews setting
unskippable_reviewers = ["@BakerNet"]
```

The `unskippable_reviewers` setting has no effect if `max_reviews` is not set.

### Ignored Directories

To prevent ownership rules from being checked or applied for certain directories, you can use `ignore` setting:

`codeowners.toml`
```toml
# `ignore` (default empty) allows you to specify directories that should be ignored by the codeowners check
ignore = ["test_project"]
```

### High Priority Labels

You can configure labels that indicate a high priority PR. When a PR has any of these labels, the comment will include a high priority indicator:

```toml
high_priority_labels = ["high-priority", "urgent"]
```

When a PR has any of these labels, the comment will look like this:
```
❗High Prio❗

Codeowners approval required for this PR:
- @user1
- @user2
```

## Quiet Mode

You can run Codeowners Plus in a "quiet" mode using the `quiet` input in the GitHub Action.

### When Quiet Mode is Enabled

* **No Comments:** The action will **not** post the review status comment (listing required/unapproved reviewers) or the optional reviewer "cc" comment to the Pull Request.
* **No Review Requests:** The action will **not** automatically request reviews from required owners who have not yet approved via the GitHub API.

### Behavior:

Even in quiet mode, the tool still performs all its internal calculations: determining required/optional owners based on file changes, checking existing approvals, and determining if the ownership rules are satisfied. The primary outcome is still the success or failure status of the associated status check (unless you've configured `enforcement.fail_check = false`).

### Use Cases:

* **Draft Pull Requests:** This is a common use case. You might want the Codeowners Plus logic to run and report a status (e.g., pending or failed) on draft PRs, but without notifying reviewers prematurely by adding comments or requesting reviews until the PR is marked "Ready for review".
* **Custom Notification Workflows:** You might prefer to handle notifications or review requests through a different mechanism and only use Codeowners Plus for the status check enforcement.

### Activation:

* **GitHub Action:** Set the `quiet` input to `'true'`.
    ```yaml
    - name: 'Codeowners Plus (Quiet)'
      uses: multimediallc/codeowners-plus@v0.1.0
      with:
        # ... other inputs ...
        quiet: 'true'
    ```

**Default:** Quiet mode is **disabled** (`false`) by default.

## CLI Tool

A CLI tool is available which provides some utilities for working with `.codeowners` files.

You can download the built tools via [releases](https://github.com/multimediallc/codeowners-plus/releases) or build from source:
```
go build -o codeowners-cli ./tools/cli/main.go
```

Available commands are:

* `unowned` to check for unowned files
* `owner` to check who owns a specific file
* `verify` to check for typos in a `.codeowners` file

## Contributing

See [CONTRIBUTING.md](https://github.com/multimediallc/codeowners-plus/blob/main/CONTRIBUTING.md)

## Future Features

* Inline ownership comments for having owners for specific functions, classes, etc.
