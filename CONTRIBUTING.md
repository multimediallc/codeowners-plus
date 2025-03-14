# CONTRIBUTING.md

## Contributing Guidelines

Thank you for considering contributing to Codeowners Plus! We welcome contributions from the community to improve and expand this project. This document outlines how to get involved.

### How to Contribute

#### Reporting Issues

* Search existing issues to see if your concern has already been raised.
* Open a new issue if you find a bug, have a suggestion, or encounter a problem.
* Provide as much detail as possible, including steps to reproduce the issue and any relevant context.

#### Running the Project Locally

> [!Note]
> Running locally still requires real Github PRs to exist that you are testing against.

```bash
go run main.go -token <your_gh_token> -dir ../chaturbate -pr <pr_num> -repo multimediallc/chaturbate -v true
```

#### Submitting Changes

* Fork the repository
* Create a new branch for your changes
* After making code changes, run `./scripts/covbadge.sh` to update the code coverage badge (this will be enforced in GHA checks)
* Commit your changes with clear and descriptive commit messages
* Push your changes to your fork
* Open a pull request against the `main` branch of this repository

### Code Style and Standards

* Follow [Go best practices](https://go.dev/doc/effective_go).
* Write clear, concise, and well-documented code.
* Include unit tests for any new functionality.

### Reviewing and Merging Changes

* All pull requests will be reviewed by maintainers.
* Address feedback promptly and communicate if additional time is needed.
* Once approved, your changes will be merged by a maintainer.

### Community

* Join discussions in the issues section to help troubleshoot or brainstorm solutions.
* Respectfully engage with others to maintain a friendly and constructive environment.
