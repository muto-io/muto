# Contributing to Muto

Thank you for your interest in contributing to Muto! This document outlines our contribution guidelines and review process.

## Code of Conduct

We are committed to providing a welcoming and inclusive environment for all contributors. Please review and abide by our Code of Conduct.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-username>/muto.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes and commit with clear messages
5. Push to your fork and open a Pull Request

## Development Workflow

### Prerequisites

- Go 1.22+
- Docker
- kind (Kubernetes in Docker)
- kubectl

### Running Tests

```bash
# Unit tests
make test-unit

# Integration tests (Kubernetes)
make test-integration-k8s

# All tests
make test-e2e
```

See [test/integration/README.md](test/integration/README.md) for comprehensive testing documentation.

## Review SLAs

We maintain Service Level Agreements (SLAs) for code review to ensure timely feedback and maintain project momentum. These SLAs apply to all pull requests from external contributors and internal team members.

### PR Review Turnaround

- **First Response**: Within 2 business days
  - A review comment, question, or acknowledgment that indicates active review has begun
- **Final Decision**: Within 5 business days
  - Approval, request for changes, or clear feedback on next steps

**Note:** Business days exclude weekends and recognized holidays. For distributed teams across timezones, we aim to have at least one reviewer available during working hours.

### Dependabot PRs

Dependabot pull requests follow an expedited, automated SLA to keep dependencies current and secure:

- **Patch Updates** (e.g., 1.2.3 → 1.2.4):
  - Automatically approved and merged if all checks pass
  - No manual review required
  - Merged within minutes of tests passing
  - Commit squashed for clean history

- **Minor Updates** (e.g., 1.2.3 → 1.3.0):
  - Automatically approved and merged if all checks pass
  - No manual review required
  - Merged within minutes of tests passing

- **Major Version Updates** (e.g., 1.2.3 → 2.0.0):
  - Flagged for manual review
  - Require explicit approval within 3 business days
  - Reviewer checks for breaking changes and compatibility

- **Security Updates**:
  - Treated as high priority regardless of version bump
  - Auto-merged if patch or minor; manual review for major versions
  - Target approval within 1 business day for major security patches

### Escalation Path

If a pull request is not reviewed within the SLA window:

1. **After 5 business days without review**: PR is automatically labeled `sla-warning`
2. **After 7 business days without review**: Automated reminder is posted to the PR and the `@muto-io/maintainers` group is mentioned
3. **If still blocked after 9 business days**: Issue is escalated to the project lead for manual intervention and discussed in the next team standup

### Team Availability Guidelines

To maintain responsive review coverage, we aim for:

- **Geographic Coverage**: At least 1 active reviewers available during each 8-hour window across major timezones

## Pull Request Guidelines

### Before Submitting

- Ensure all tests pass locally: `make test-e2e`
- Keep commits focused and logically organized
- Write clear, descriptive commit messages
- Update documentation if your changes affect user-facing behavior

### PR Description

Include the following in your PR description:

- **What**: Brief summary of the change
- **Why**: Motivation and context for the change
- **How**: Key implementation details (if non-obvious)
- **Testing**: How to verify the change works
- **Checklist**:
  - [ ] Tests pass locally
  - [ ] Documentation updated (if applicable)
  - [ ] No breaking changes (or clearly documented if intentional)

### Commit Messages

Follow conventional commits format:

```
type(scope): subject

body

footer
```

Examples:
- `feat: Add priority queue support`
- `fix: Resolve pod reconciliation race condition`
- `docs: Update AgentJob CRD examples`
- `feat: Bump sigs.k8s.io/controller-runtime`

## Code Review Process

### What Reviewers Look For

- **Correctness**: Does the code work as intended?
- **Testing**: Are there sufficient test cases for new behavior?
- **Performance**: Could this introduce performance regressions?
- **Security**: Are there potential security concerns?
- **Maintainability**: Is the code clear and maintainable?

### Author Response

When addressing feedback:

1. Acknowledge each piece of feedback
2. Make necessary changes or explain why a suggestion isn't adopted
3. Reply to each review comment
4. Request re-review after making changes

## Deployment & Releases

Muto follows semantic versioning (MAJOR.MINOR.PATCH). Releases are created by maintainers and published to:

- GitHub Releases
- Container registries (Docker Hub, GHCR)
- Go module repositories

Release notes should summarize breaking changes, new features, and bug fixes.

## Reporting Issues

When reporting bugs, please include:

- Go version (`go version`)
- Kubernetes/Cloud Foundry version (if applicable)
- Steps to reproduce
- Expected vs. actual behavior
- Relevant logs or error messages

## Questions?

- Check existing [issues](https://github.com/muto-io/muto/issues) and [discussions](https://github.com/muto-io/muto/discussions)
- Open a new discussion for questions or ideas
- Reach out to the maintainers team for urgent concerns

Thank you for contributing to Muto!
